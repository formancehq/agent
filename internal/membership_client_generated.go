// Code generated by MockGen. DO NOT EDIT.
// Source: /src/ee/agent/internal/membership_listener.go
//
// Generated by this command:
//
//	mockgen -source=/src/ee/agent/internal/membership_listener.go -destination=/src/ee/agent/internal/membership_client_generated.go -package=internal . MembershipClient
//

// Package internal is a generated GoMock package.
package internal

import (
	reflect "reflect"

	generated "github.com/formancehq/stack/components/agent/internal/generated"
	gomock "go.uber.org/mock/gomock"
)

// MockMembershipClient is a mock of MembershipClient interface.
type MockMembershipClient struct {
	ctrl     *gomock.Controller
	recorder *MockMembershipClientMockRecorder
}

// MockMembershipClientMockRecorder is the mock recorder for MockMembershipClient.
type MockMembershipClientMockRecorder struct {
	mock *MockMembershipClient
}

// NewMockMembershipClient creates a new mock instance.
func NewMockMembershipClient(ctrl *gomock.Controller) *MockMembershipClient {
	mock := &MockMembershipClient{ctrl: ctrl}
	mock.recorder = &MockMembershipClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMembershipClient) EXPECT() *MockMembershipClientMockRecorder {
	return m.recorder
}

// Orders mocks base method.
func (m *MockMembershipClient) Orders() chan *generated.Order {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Orders")
	ret0, _ := ret[0].(chan *generated.Order)
	return ret0
}

// Orders indicates an expected call of Orders.
func (mr *MockMembershipClientMockRecorder) Orders() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Orders", reflect.TypeOf((*MockMembershipClient)(nil).Orders))
}

// Send mocks base method.
func (m *MockMembershipClient) Send(message *generated.Message) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Send", message)
	ret0, _ := ret[0].(error)
	return ret0
}

// Send indicates an expected call of Send.
func (mr *MockMembershipClientMockRecorder) Send(message any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Send", reflect.TypeOf((*MockMembershipClient)(nil).Send), message)
}