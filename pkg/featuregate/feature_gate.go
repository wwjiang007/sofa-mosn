/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package featuregate

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/pflag"
	"mosn.io/mosn/pkg/log"
)

type Feature string

const (
	flagName = "feature-gates"

	// allAlphaGate is a global toggle for alpha features. Per-feature key
	// values override the default set by allAlphaGate. Examples:
	//   AllAlpha=false,NewFeature=true  will result in newFeature=true
	//   AllAlpha=true,NewFeature=false  will result in newFeature=false
	allAlphaGate Feature = "AllAlpha"

	mapStringBool = "mapStringBool"
)

var (
	// The generic features.
	defaultFeatures = map[Feature]FeatureSpec{
		allAlphaGate: {Default: false, PreRelease: Alpha},
	}

	// Special handling for a few gates.
	specialFeatures = map[Feature]func(known map[Feature]FeatureSpec, enabled map[Feature]bool, val bool){
		allAlphaGate: setUnsetAlphaGates,
	}
)

type FeatureSpec struct {
	// Default is the default enablement state for the feature
	Default bool
	// LockToDefault indicates that the feature is locked to its default and cannot be changed
	LockToDefault bool
	// PreRelease indicates the maturity level of the feature
	PreRelease prerelease
	// InitFunc used to init process when StartInit is invoked
	InitFunc func()
}

type prerelease string

const (
	// Values for PreRelease.
	Alpha = prerelease("ALPHA")
	Beta  = prerelease("BETA")
	GA    = prerelease("")
)

type oneTimeBroadcaster struct {
	notified bool
	once     sync.Once
	channel  chan struct{}
}

// FeatureGate indicates whether a given feature is enabled or not
type FeatureGate interface {
	// Enabled returns true if the key is enabled.
	Enabled(key Feature) bool
	// KnownFeatures returns a slice of strings describing the FeatureGate's known features.
	KnownFeatures() []string
	// DeepCopy returns a deep copy of the FeatureGate object, such that gates can be
	// set on the copy without mutating the original. This is useful for validating
	// config against potential feature gate changes before committing those changes.
	DeepCopy() MutableFeatureGate

	// IsReady returns true if the feature is ready
	IsReady(key Feature) bool
	// Subscribe returns a channel. if the channel is closed, means the feature is ready
	Subscribe(key Feature) (chan struct{}, error)
}

// MutableFeatureGate parses and stores flag gates for known features from
// a string like feature1=true,feature2=false,...
type MutableFeatureGate interface {
	FeatureGate

	// AddFlag adds a flag for setting global feature gates to the specified FlagSet.
	AddFlag(fs *pflag.FlagSet)
	// Set parses and stores flag gates for known features
	// from a string like feature1=true,feature2=false,...
	Set(value string) error
	// SetFromMap stores flag gates for known features from a map[string]bool or returns an error
	SetFromMap(m map[string]bool) error
	// Add adds features to the featureGate.
	Add(features map[Feature]FeatureSpec) error
	// AddFeatureSpec adds feature to the featureGate.
	AddFeatureSpec(key Feature, spec FeatureSpec) error
	// call StartInit to trigger init functions of feature
	StartInit() error
}

// defaultFeatureGate implements FeatureGate as well as pflag.Value for flag parsing.
type defaultFeatureGate struct {
	special map[Feature]func(map[Feature]FeatureSpec, map[Feature]bool, bool)

	// lock guards writes to known, enabled, and reads/writes of closed
	lock sync.Mutex
	// known holds a map[Feature]FeatureSpec
	known *atomic.Value
	// enabled holds a map[Feature]bool
	enabled *atomic.Value
	// closed is set to true when AddFlag is called, and prevents subsequent calls to Add
	closed bool
	// ready holds a map[Feature]bool
	ready *atomic.Value
	// using to notify subscriber
	broadcasters map[Feature]*oneTimeBroadcaster
	// feature gate status info
	info string
}

func setUnsetAlphaGates(known map[Feature]FeatureSpec, enabled map[Feature]bool, val bool) {
	for k, v := range known {
		if v.PreRelease == Alpha {
			if _, found := enabled[k]; !found {
				enabled[k] = val
			}
		}
	}
}

// Set, String, and Type implement pflag.Value
var _ pflag.Value = &defaultFeatureGate{}

func NewFeatureGate() *defaultFeatureGate {
	known := map[Feature]FeatureSpec{}
	for k, v := range defaultFeatures {
		known[k] = v
	}

	knownValue := &atomic.Value{}
	knownValue.Store(known)

	enabled := map[Feature]bool{}
	enabledValue := &atomic.Value{}
	enabledValue.Store(enabled)

	ready := map[Feature]bool{}
	readyValue := &atomic.Value{}
	readyValue.Store(ready)

	f := &defaultFeatureGate{
		known:        knownValue,
		special:      specialFeatures,
		enabled:      enabledValue,
		ready:        readyValue,
		broadcasters: map[Feature]*oneTimeBroadcaster{},
	}
	return f
}

// Set parses a string of the form "key1=value1,key2=value2,..." into a
// map[string]bool of known keys or returns an error.
func (f *defaultFeatureGate) Set(value string) error {
	m := make(map[string]bool)
	for _, s := range strings.Split(value, ",") {
		if len(s) == 0 {
			continue
		}
		arr := strings.SplitN(s, "=", 2)
		k := strings.TrimSpace(arr[0])
		if len(arr) != 2 {
			return fmt.Errorf("missing bool value for %s", k)
		}
		v := strings.TrimSpace(arr[1])
		boolValue, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid value of %s=%s, err: %v", k, v, err)
		}
		m[k] = boolValue
	}
	return f.SetFromMap(m)
}

// SetFromMap stores flag gates for known features from a map[string]bool or returns an error
func (f *defaultFeatureGate) SetFromMap(m map[string]bool) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	// Copy existing state
	known := map[Feature]FeatureSpec{}
	for k, v := range f.known.Load().(map[Feature]FeatureSpec) {
		known[k] = v
	}
	enabled := map[Feature]bool{}
	for k, v := range f.enabled.Load().(map[Feature]bool) {
		enabled[k] = v
	}

	for k, v := range m {
		k := Feature(k)
		featureSpec, ok := known[k]
		if !ok {
			return fmt.Errorf("unrecognized feature gate: %s", k)
		}
		if featureSpec.LockToDefault && featureSpec.Default != v {
			return fmt.Errorf("cannot set feature gate %v to %v, feature is locked to %v", k, v, featureSpec.Default)
		}
		enabled[k] = v
		// Handle "special" features like "all alpha gates"
		if fn, found := f.special[k]; found {
			fn(known, enabled, v)
		}

		if featureSpec.PreRelease == GA {
			log.DefaultLogger.Warnf("Setting GA feature gate %s=%t. It will be removed in a future release.", k, v)
		}
	}

	// Persist changes
	f.known.Store(known)
	f.enabled.Store(enabled)

	// set info
	pairs := []string{}
	for k, v := range enabled {
		pairs = append(pairs, fmt.Sprintf("%s=%t", k, v))
	}
	sort.Strings(pairs)
	f.info = strings.Join(pairs, ",")

	log.DefaultLogger.Infof("feature gates: %v", f.enabled)
	return nil
}

// String returns a string containing all enabled feature gates, formatted as "key1=value1,key2=value2,...".
func (f *defaultFeatureGate) String() string {
	return f.info
}

func (f *defaultFeatureGate) Type() string {
	return mapStringBool
}

// Add adds features to the featureGate.
func (f *defaultFeatureGate) Add(features map[Feature]FeatureSpec) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return fmt.Errorf("cannot add a feature gate after adding it to the flag set")
	}

	// Copy existing state
	known := map[Feature]FeatureSpec{}
	for k, v := range f.known.Load().(map[Feature]FeatureSpec) {
		known[k] = v
	}

	for name, spec := range features {
		if existingSpec, found := known[name]; found {
			if reflect.DeepEqual(existingSpec, spec) {
				continue
			}
			return fmt.Errorf("feature gate %q with different spec already exists: %v", name, existingSpec)
		}

		known[name] = spec

		if _, found := f.broadcasters[name]; !found {
			f.broadcasters[name] = &oneTimeBroadcaster{
				notified: false,
				channel:  make(chan struct{}, 1),
			}
		}
	}

	// Persist updated state
	f.known.Store(known)

	return nil
}

// Enabled returns true if the key is enabled.
func (f *defaultFeatureGate) Enabled(key Feature) bool {
	if v, ok := f.enabled.Load().(map[Feature]bool)[key]; ok {
		return v
	}
	if v, ok := f.known.Load().(map[Feature]FeatureSpec)[key]; ok {
		return v.Default
	}
	return false
}

// AddFlag adds a flag for setting global feature gates to the specified FlagSet.
func (f *defaultFeatureGate) AddFlag(fs *pflag.FlagSet) {
	f.lock.Lock()
	// TODO(mtaufen): Shouldn't we just close it on the first Set/SetFromMap instead?
	// Not all components expose a feature gates flag using this AddFlag method, and
	// in the future, all components will completely stop exposing a feature gates flag,
	// in favor of componentconfig.
	f.closed = true
	f.lock.Unlock()

	known := f.KnownFeatures()
	fs.Var(f, flagName, ""+
		"A set of key=value pairs that describe feature gates for alpha/experimental features. "+
		"Options are:\n"+strings.Join(known, "\n"))
}

// KnownFeatures returns a slice of strings describing the FeatureGate's known features.
// Deprecated and GA features are hidden from the list.
func (f *defaultFeatureGate) KnownFeatures() []string {
	var known []string
	for k, v := range f.known.Load().(map[Feature]FeatureSpec) {
		if v.PreRelease == GA {
			continue
		}
		known = append(known, fmt.Sprintf("%s=true|false (%s - default=%t)", k, v.PreRelease, v.Default))
	}
	sort.Strings(known)
	return known
}

// DeepCopy returns a deep copy of the FeatureGate object, such that gates can be
// set on the copy without mutating the original. This is useful for validating
// config against potential feature gate changes before committing those changes.
func (f *defaultFeatureGate) DeepCopy() MutableFeatureGate {
	// Copy existing state.
	known := map[Feature]FeatureSpec{}
	for k, v := range f.known.Load().(map[Feature]FeatureSpec) {
		known[k] = v
	}
	enabled := map[Feature]bool{}
	for k, v := range f.enabled.Load().(map[Feature]bool) {
		enabled[k] = v
	}
	ready := map[Feature]bool{}
	for k, v := range f.ready.Load().(map[Feature]bool) {
		enabled[k] = v
	}
	broadcasters := map[Feature]*oneTimeBroadcaster{}
	for k, v := range f.broadcasters {
		broadcasters[k] = &oneTimeBroadcaster{
			notified: v.notified,
			channel:  v.channel,
		}
	}

	// Store copied state in new atomics.
	knownValue := &atomic.Value{}
	knownValue.Store(known)
	enabledValue := &atomic.Value{}
	enabledValue.Store(enabled)
	readyValue := &atomic.Value{}
	readyValue.Store(ready)

	// Construct a new featureGate around the copied state.
	// Note that specialFeatures is treated as immutable by convention,
	// and we maintain the value of f.closed across the copy.
	return &defaultFeatureGate{
		special:      specialFeatures,
		known:        knownValue,
		enabled:      enabledValue,
		closed:       f.closed,
		ready:        readyValue,
		broadcasters: broadcasters,
	}
}

// UpdateToReady supports updating feature to ready, then broadcasting the ReadMessage to subscribers
// unsupported:
// 1. Rollback the notified of ready to false or Setting with false
// 2. Repeat setting the notified of ready with true
func (f *defaultFeatureGate) updateToReady(key Feature) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	if _, ok := f.known.Load().(map[Feature]FeatureSpec)[key]; !ok {
		return fmt.Errorf("feature %s is unknown", key)
	}

	if !f.Enabled(key) {
		return fmt.Errorf("feature %s is disabled, can not update to ready", key)
	}

	if b, found := f.broadcasters[key]; found {
		if b.notified {
			return fmt.Errorf("repeat setting feature %s to ready", key)
		}

		b.notified = true
		b.once.Do(func() {
			log.DefaultLogger.Infof("feature %s is ready, notify subscriber", key)
			close(b.channel)
		})

		f.doUpdateToReady(key)
	} else {
		return fmt.Errorf("unrecognized feature gate: %s", key)
	}

	return nil
}

func (f *defaultFeatureGate) doUpdateToReady(key Feature) {
	// Copy existing state
	ready := map[Feature]bool{}
	for k, v := range f.ready.Load().(map[Feature]bool) {
		ready[k] = v
	}
	ready[key] = true
	f.ready.Store(ready)
}

// IsReady returns true if the feature is ready
func (f *defaultFeatureGate) IsReady(key Feature) bool {
	if v, ok := f.ready.Load().(map[Feature]bool)[key]; ok {
		return v
	}

	return false
}

// Subscribe returns a channel. if the channel is closed, means the feature is ready
func (f *defaultFeatureGate) Subscribe(key Feature) (chan struct{}, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if _, ok := f.known.Load().(map[Feature]FeatureSpec)[key]; !ok {
		return nil, fmt.Errorf("feature %s is unknown", key)
	}

	if broadcaster, found := f.broadcasters[key]; found {
		return broadcaster.channel, nil
	}

	return nil, fmt.Errorf("subscribe fails, make sure %s is inited correctly", key)
}

// AddFeatureSpec adds feature to the featureGate.
func (f *defaultFeatureGate) AddFeatureSpec(key Feature, spec FeatureSpec) error {
	return f.Add(map[Feature]FeatureSpec{key: spec})
}

// call StartInit to trigger init functions of feature
func (f *defaultFeatureGate) StartInit() error {
	w := &sync.WaitGroup{}
	for key, enabled := range f.enabled.Load().(map[Feature]bool) {
		if !enabled {
			continue
		}

		spec, _ := f.known.Load().(map[Feature]FeatureSpec)[key]

		w.Add(1)
		log.DefaultLogger.Infof("feature %s start init", key)
		go func(wg *sync.WaitGroup, k Feature) {
			if spec.InitFunc != nil {
				spec.InitFunc()
			}
			f.updateToReady(k)
			log.DefaultLogger.Infof("feature %s init done", k)
			wg.Done()
		}(w, key)
	}

	w.Wait()
	return nil
}
