package tests

import (
    "encoding/json"
    "net/http"
    "net/url"
    "testing"
    "time"

    "progressdb/pkg/models"
    "progressdb/pkg/store"
    utils "progressdb/tests/utils"
)

func TestAuthorization_AdminCanAccessDeletedThread(t *testing.T) {
    srv := utils.SetupServer(t)
    defer srv.Close()

    // create a soft-deleted thread in the store
    th := models.Thread{
        ID:        "auth-thread-1",
        Title:     "t",
        Author:    "alice",
        Deleted:   true,
        DeletedTS: time.Now().Add(-24 * time.Hour).UnixNano(),
    }
    b, _ := json.Marshal(th)
    if err := store.SaveThread(th.ID, string(b)); err != nil {
        t.Fatalf("SaveThread: %v", err)
    }

    // admin request supplying author via query param
    q := url.Values{}
    q.Set("author", "alice")
    req, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+th.ID+"?"+q.Encode(), nil)
    req.Header.Set("X-Role-Name", "admin")
    res, _ := http.DefaultClient.Do(req)
    if res.StatusCode != 200 {
        t.Fatalf("expected admin to access deleted thread; status=%d", res.StatusCode)
    }

    // signed request as the original author should still be treated as not found (soft-deleted)
    sig := utils.SignHMAC("signsecret", "alice")
    sreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+th.ID+"", nil)
    sreq.Header.Set("X-User-ID", "alice")
    sreq.Header.Set("X-User-Signature", sig)
    sres, _ := http.DefaultClient.Do(sreq)
    if sres.StatusCode == 200 {
        t.Fatalf("expected signed non-admin to not see deleted thread; got 200")
    }
}

