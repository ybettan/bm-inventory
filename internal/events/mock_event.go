// Code generated by MockGen. DO NOT EDIT.
// Source: event.go

// Package events is a generated GoMock package.
package events

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
	time "time"
)

// MockHandler is a mock of Handler interface
type MockHandler struct {
	ctrl     *gomock.Controller
	recorder *MockHandlerMockRecorder
}

// MockHandlerMockRecorder is the mock recorder for MockHandler
type MockHandlerMockRecorder struct {
	mock *MockHandler
}

// NewMockHandler creates a new mock instance
func NewMockHandler(ctrl *gomock.Controller) *MockHandler {
	mock := &MockHandler{ctrl: ctrl}
	mock.recorder = &MockHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockHandler) EXPECT() *MockHandlerMockRecorder {
	return m.recorder
}

// AddEvent mocks base method
func (m *MockHandler) AddEvent(ctx context.Context, entityID, severity, msg string, eventTime time.Time, otherEntities ...string) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, entityID, severity,msg, eventTime}
	for _, a := range otherEntities {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "AddEvent", varargs...)
}

// AddEvent indicates an expected call of AddEvent
func (mr *MockHandlerMockRecorder) AddEvent(ctx, entityID, severity, msg, eventTime interface{}, otherEntities ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, entityID, severity, msg, eventTime}, otherEntities...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddEvent", reflect.TypeOf((*MockHandler)(nil).AddEvent), varargs...)
}

// GetEvents mocks base method
func (m *MockHandler) GetEvents(entityID string) ([]*Event, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetEvents", entityID)
	ret0, _ := ret[0].([]*Event)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetEvents indicates an expected call of GetEvents
func (mr *MockHandlerMockRecorder) GetEvents(entityID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetEvents", reflect.TypeOf((*MockHandler)(nil).GetEvents), entityID)
}
