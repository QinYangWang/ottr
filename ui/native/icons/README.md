# Icons

Every icon here is Lucide's, taken unmodified from
[`lucide-static`](https://unpkg.com/lucide-static@0.468.0/icons/) v0.468.0, and
licensed under the ISC License in [LICENSE](LICENSE).

| File | Lucide name | Used for |
|---|---|---|
| `monitor.svg` | monitor | the whole screen, and each display |
| `app-window.svg` | app-window | recording a window |
| `scan.svg` | scan | recording an area (not yet) |
| `video.svg` | video | recording a camera (not yet) |
| `mic.svg` | mic | the microphone |
| `volume-2.svg` | volume-2 | system audio |
| `import.svg` | import | adding a file to the library |
| `sliders-horizontal.svg` | sliders-horizontal | settings |
| `arrow-left.svg` | arrow-left | back |
| `check.svg` | check | the source chosen |
| `languages.svg` | languages | translation settings |
| `file-text.svg` | file-text | summary settings |
| `library.svg` | library | opening Library |
| `sun.svg` / `moon.svg` | sun / moon | switching light and dark themes |

`icons.go` embeds them and parses each once with `ui.MustParseSVG`; `ui.Icon`
paints the shapes in the color of the text around them, so the SVGs' own
`stroke="currentColor"` is what keeps them looking right anywhere.
