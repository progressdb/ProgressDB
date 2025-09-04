package banner

import (
    "fmt"

    fig "github.com/mbndr/figlet4go"
)

// Print renders a simple 3D-styled banner and basic info.
func Print(addr, dbPath string) {
    r := fig.NewAsciiRender()

    // Try a 3D-ish font; fall back gracefully if not available.
    opts := fig.NewRenderOptions()
    opts.FontName = "3-d" // alternatives: "block", "3x5", "electronic"
    opts.FontColor = []fig.Color{fig.ColorCyan, fig.ColorYellow}

    if out, err := r.RenderOpts("PROGRESS DB", opts); err == nil {
        fmt.Print(out)
    } else if out, err := r.Render("PROGRESS DB"); err == nil { // default font
        fmt.Print(out)
    } else {
        // Last resort plain text
        fmt.Println("PROGRESS DB")
    }

    fmt.Printf("Listening: http://localhost%s\n", addr)
    fmt.Printf("DB:        %s\n", dbPath)
    fmt.Println("Endpoints:")
    fmt.Println("  GET  /?thread=<id>&msg=<text>")
    fmt.Println("  GET  /?thread=<id>")
    fmt.Println("  POST / (form: thread,msg)")
}

