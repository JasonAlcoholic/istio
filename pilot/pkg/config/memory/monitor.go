// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"istio.io/istio/galley/pkg/config/schema/resource"
	"istio.io/pkg/log"

	"istio.io/istio/pilot/pkg/model"
)

const (
	// BufferSize specifies the buffer size of event channel
	BufferSize = 100
)

// Handler specifies a function to apply on a Config for a given event type
type Handler func(model.Config, model.Config, model.Event)

// Monitor provides methods of manipulating changes in the config store
type Monitor interface {
	Run(<-chan struct{})
	AppendEventHandler(resource.GroupVersionKind, Handler)
	ScheduleProcessEvent(ConfigEvent)
}

// ConfigEvent defines the event to be processed
type ConfigEvent struct {
	config model.Config
	old    model.Config
	event  model.Event
}

type configstoreMonitor struct {
	store    model.ConfigStore
	handlers map[resource.GroupVersionKind][]Handler
	eventCh  chan ConfigEvent
}

// NewMonitor returns new Monitor implementation with a default event buffer size.
func NewMonitor(store model.ConfigStore) Monitor {
	return NewBufferedMonitor(store, BufferSize)
}

// NewBufferedMonitor returns new Monitor implementation with the specified event buffer size
func NewBufferedMonitor(store model.ConfigStore, bufferSize int) Monitor {
	handlers := make(map[resource.GroupVersionKind][]Handler)

	for _, s := range store.Schemas().All() {
		handlers[s.Resource().GroupVersionKind()] = make([]Handler, 0)
	}

	return &configstoreMonitor{
		store:    store,
		handlers: handlers,
		eventCh:  make(chan ConfigEvent, bufferSize),
	}
}

func (m *configstoreMonitor) ScheduleProcessEvent(configEvent ConfigEvent) {
	m.eventCh <- configEvent
}

func (m *configstoreMonitor) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			if _, ok := <-m.eventCh; ok {
				close(m.eventCh)
			}
			return
		case ce, ok := <-m.eventCh:
			if ok {
				m.processConfigEvent(ce)
			}
		}
	}
}

func (m *configstoreMonitor) processConfigEvent(ce ConfigEvent) {
	if _, exists := m.handlers[ce.config.GroupVersionKind()]; !exists {
		log.Warnf("Config Type %s does not exist in config store", ce.config.Type)
		return
	}
	m.applyHandlers(ce.old, ce.config, ce.event)
}

func (m *configstoreMonitor) AppendEventHandler(typ resource.GroupVersionKind, h Handler) {
	m.handlers[typ] = append(m.handlers[typ], h)
}

func (m *configstoreMonitor) applyHandlers(old model.Config, config model.Config, e model.Event) {
	for _, f := range m.handlers[config.GroupVersionKind()] {
		f(old, config, e)
	}
}
