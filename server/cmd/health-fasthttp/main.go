package main

import (
    "flag"
    "fmt"
    "time"

    "github.com/valyala/fasthttp"
)

func main() {
    addr := flag.String("addr", ":8081", "listen address for fasthttp health POC")
    ver := flag.String("version", "dev", "version string to return")
    flag.Parse()

    h := func(ctx *fasthttp.RequestCtx) {
        switch string(ctx.Path()) {
        case "/health", "/healthz":
            ctx.Response.Header.Set("Content-Type", "application/json")
            ctx.SetStatusCode(fasthttp.StatusOK)
            // keep the handler extremely lean to measure router+net overhead
            _, _ = ctx.WriteString(fmt.Sprintf("{\"status\":\"ok\",\"version\":\"%s\"}", *ver))
        default:
            ctx.SetStatusCode(fasthttp.StatusNotFound)
        }
    }

    fmt.Printf("fasthttp health POC listening on %s\n", *addr)
    // tune server options for high throughput
    srv := &fasthttp.Server{
        Handler:            h,
        Name:               "progressdb-fasthttp-poc",
        ReadTimeout:        5 * time.Second,
        WriteTimeout:       5 * time.Second,
        MaxRequestBodySize: 1 << 20,
    }
    if err := srv.ListenAndServe(*addr); err != nil {
        fmt.Printf("fasthttp server exit: %v\n", err)
    }
}

