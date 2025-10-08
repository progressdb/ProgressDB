package hardware

import (
    "testing"
    "time"
)

func TestSensorBasic(t *testing.T) {
    s := NewSensor(50 * time.Millisecond)
    s.Start()
    defer s.Stop()

    // wait for at least one sample
    time.Sleep(120 * time.Millisecond)
    snap := s.Snapshot()
    if snap.Timestamp.IsZero() {
        t.Fatalf("expected non-zero snapshot timestamp")
    }

    // register a throttle handler and ensure SendThrottle reaches it
    ch := make(chan ThrottleRequest, 1)
    s.RegisterThrottleHandler(func(r ThrottleRequest) {
        ch <- r
    })

    req := ThrottleRequest{Source: "test", Reason: "unit", Severity: 0.5}
    s.SendThrottle(req)

    select {
    case r := <-ch:
        if r.Source != "test" || r.Reason != "unit" {
            t.Fatalf("unexpected throttle request: %+v", r)
        }
    case <-time.After(500 * time.Millisecond):
        t.Fatalf("throttle handler not invoked")
    }
}

