package worker

import (
	"errors"
	"sync"
	"testing"
	"time"

	pbs "github.com/hashicorp/boundary/internal/gen/controller/servers/services"
	"github.com/hashicorp/boundary/internal/session"
	"github.com/stretchr/testify/require"
)

func TestWorkerMakeCloseConnectionRequest(t *testing.T) {
	require := require.New(t)
	in := map[string]string{"foo": "one", "bar": "two"}
	expected := &pbs.CloseConnectionRequest{
		CloseRequestData: []*pbs.CloseConnectionRequestData{
			{ConnectionId: "foo", Reason: session.UnknownReason.String()},
			{ConnectionId: "bar", Reason: session.UnknownReason.String()},
		},
	}
	actual := new(Worker).makeCloseConnectionRequest(in)
	require.ElementsMatch(expected.GetCloseRequestData(), actual.GetCloseRequestData())
}

func TestMakeSessionCloseInfo(t *testing.T) {
	require := require.New(t)
	closeInfo := map[string]string{"foo": "one", "bar": "two"}
	response := &pbs.CloseConnectionResponse{
		CloseResponseData: []*pbs.CloseConnectionResponseData{
			{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
			{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
		},
	}
	expected := map[string][]*pbs.CloseConnectionResponseData{
		"one": []*pbs.CloseConnectionResponseData{
			{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
		},
		"two": []*pbs.CloseConnectionResponseData{
			{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
		},
	}
	actual := new(Worker).makeSessionCloseInfo(closeInfo, response)
	require.Equal(expected, actual)
}

func TestMakeSessionCloseInfoPanicIfCloseInfoNil(t *testing.T) {
	require := require.New(t)
	require.Panics(func() {
		new(Worker).makeSessionCloseInfo(nil, nil)
	})
}

func TestMakeSessionCloseInfoEmpty(t *testing.T) {
	require := require.New(t)
	require.Equal(
		make(map[string][]*pbs.CloseConnectionResponseData),
		new(Worker).makeSessionCloseInfo(make(map[string]string), nil),
	)
}

func TestWorkerSetCloseTimeForResponse(t *testing.T) {
	cases := []struct {
		name             string
		sessionCloseInfo map[string][]*pbs.CloseConnectionResponseData
		sessionInfoMap   func() *sync.Map
		expected         []string
		expectedClosed   map[string]struct{}
		expectedErr      []error
	}{
		{
			name: "basic",
			sessionCloseInfo: map[string][]*pbs.CloseConnectionResponseData{
				"one": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
				"two": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
			},
			sessionInfoMap: func() *sync.Map {
				m := new(sync.Map)
				m.Store("one", &sessionInfo{
					id: "one",
					connInfoMap: map[string]*connInfo{
						"foo": &connInfo{id: "foo"},
					},
				})
				m.Store("two", &sessionInfo{
					id: "two",
					connInfoMap: map[string]*connInfo{
						"bar": &connInfo{id: "bar"},
					},
				})
				m.Store("three", &sessionInfo{
					id: "three",
					connInfoMap: map[string]*connInfo{
						"baz": &connInfo{id: "baz"},
					},
				})

				return m
			},
			expected: []string{"foo", "bar"},
			expectedClosed: map[string]struct{}{
				"foo": struct{}{},
				"bar": struct{}{},
			},
		},
		{
			name: "not closed",
			sessionCloseInfo: map[string][]*pbs.CloseConnectionResponseData{
				"one": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
				"two": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CONNECTED},
				},
			},
			sessionInfoMap: func() *sync.Map {
				m := new(sync.Map)
				m.Store("one", &sessionInfo{
					id: "one",
					connInfoMap: map[string]*connInfo{
						"foo": &connInfo{id: "foo"},
					},
				})
				m.Store("two", &sessionInfo{
					id: "two",
					connInfoMap: map[string]*connInfo{
						"bar": &connInfo{id: "bar"},
					},
				})

				return m
			},
			expected: []string{"foo"},
			expectedClosed: map[string]struct{}{
				"foo": struct{}{},
			},
		},
		{
			name: "missing session",
			sessionCloseInfo: map[string][]*pbs.CloseConnectionResponseData{
				"one": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
				"two": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
			},
			sessionInfoMap: func() *sync.Map {
				m := new(sync.Map)
				m.Store("one", &sessionInfo{
					id: "one",
					connInfoMap: map[string]*connInfo{
						"foo": &connInfo{id: "foo"},
					},
				})

				return m
			},
			expected: []string{"foo"},
			expectedClosed: map[string]struct{}{
				"foo": struct{}{},
			},
			expectedErr: []error{
				errors.New(`could not find session ID "two" in local state after closing connections`),
			},
		},
		{
			name: "missing connection",
			sessionCloseInfo: map[string][]*pbs.CloseConnectionResponseData{
				"one": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "foo", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
				"two": []*pbs.CloseConnectionResponseData{
					{ConnectionId: "bar", Status: pbs.CONNECTIONSTATUS_CONNECTIONSTATUS_CLOSED},
				},
			},
			sessionInfoMap: func() *sync.Map {
				m := new(sync.Map)
				m.Store("one", &sessionInfo{
					id: "one",
					connInfoMap: map[string]*connInfo{
						"foo": &connInfo{id: "foo"},
					},
				})
				m.Store("two", &sessionInfo{id: "two"})

				return m
			},
			expected: []string{"foo"},
			expectedClosed: map[string]struct{}{
				"foo": struct{}{},
			},
			expectedErr: []error{
				errors.New(`could not find connection ID "bar" for session ID "two" in local state after closing connections`),
			},
		},
		{
			name:             "empty",
			sessionCloseInfo: make(map[string][]*pbs.CloseConnectionResponseData),
			sessionInfoMap: func() *sync.Map {
				m := new(sync.Map)
				m.Store("one", &sessionInfo{
					id: "one",
					connInfoMap: map[string]*connInfo{
						"foo": &connInfo{id: "foo"},
					},
				})

				return m
			},
			expected:       []string{},
			expectedClosed: map[string]struct{}{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)
			w := &Worker{
				sessionInfoMap: tc.sessionInfoMap(),
			}
			actual, actualErr := w.setCloseTimeForResponse(tc.sessionCloseInfo)

			// Assert all close times were set
			w.sessionInfoMap.Range(func(key, value interface{}) bool {
				t.Helper()
				for _, ci := range value.(*sessionInfo).connInfoMap {
					if _, ok := tc.expectedClosed[ci.id]; ok {
						require.NotEqual(time.Time{}, ci.closeTime)
					} else {
						require.Equal(time.Time{}, ci.closeTime)
					}
				}

				return true
			})

			// Assert return values
			require.ElementsMatch(tc.expected, actual)
			require.ElementsMatch(tc.expectedErr, actualErr)
		})
	}
}
