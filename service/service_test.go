package service

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestService(t *testing.T) {
	defaultStartSleep := 5 * time.Millisecond

	cases := []struct {
		testDescription string
		startServices   []*testService
		stopServices    []*testService
		expectError     bool
	}{
		{
			testDescription: "No errors on start or stop",
			startServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, nil, defaultStartSleep),
			},
			stopServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, nil, defaultStartSleep),
			},
			expectError: false,
		},
		{
			testDescription: "Errors on all start and stop",
			startServices: []*testService{
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
			},
			stopServices: []*testService{
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
			},
			expectError: true,
		},
		{
			testDescription: "One error on stop",
			startServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, nil, defaultStartSleep),
			},
			stopServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
			},
			expectError: true,
		},
		{
			testDescription: "One error on start",
			startServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, fmt.Errorf("fake error"), defaultStartSleep),
			},
			stopServices: []*testService{
				newTestService(t, nil, defaultStartSleep),
				newTestService(t, nil, defaultStartSleep),
			},
			expectError: true,
		},
	}

	for i, c := range cases {
		t.Logf("Test iteration %d: %s", i, c.testDescription)

		errGroup, ctx, cancel := NewErrGroupAndContext()
		defer cancel()

		for i := range c.startServices {
			Start(ctx, errGroup, c.startServices[i])
		}

		timeoutCtx, timeoutCancel := NewShutdownTimeoutContext()
		defer timeoutCancel()

		for i := range c.stopServices {
			Stop(timeoutCtx, errGroup, c.stopServices[i])
		}

		err := WaitForErrGroup(errGroup)
		if !c.expectError {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestServiceStartTimeout(t *testing.T) {
	timeout := 5 * time.Millisecond
	testService := newTestService(t, nil, timeout*2)
	errGroup, ctx, cancel := NewErrGroupAndContext()
	defer cancel()

	start(ctx, errGroup, testService, timeout)
	err := WaitForErrGroup(errGroup)
	require.Error(t, err)
}

func TestWaitForStopContext(t *testing.T) {
	stopCh := NewStopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	stoppedBy := WaitForStop(stopCh, ctx)
	require.Equal(t, "context", stoppedBy)
}

func TestWaitForStopChannel(t *testing.T) {
	stopCh := NewStopChannel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(5 * time.Millisecond)
		stopCh <- syscall.SIGINT
	}()

	stoppedBy := WaitForStop(stopCh, ctx)
	require.Equal(t, "os.Signal (interrupt)", stoppedBy)
}

type testService struct {
	t      *testing.T
	result error
	sleep  time.Duration
}

func newTestService(t *testing.T, result error, sleep time.Duration) *testService {
	return &testService{
		t:      t,
		result: result,
		sleep:  sleep,
	}
}

func (svc *testService) Start(ctx context.Context, wg *sync.WaitGroup) error {
	svc.t.Helper()

	time.Sleep(svc.sleep)

	wg.Done()

	return svc.result
}

func (svc *testService) Stop(ctx context.Context) error {
	svc.t.Helper()
	return svc.result
}
