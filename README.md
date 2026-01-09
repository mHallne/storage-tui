# storage-tui

Azure Storage Explorer TUI built with tview. This is a foundation with mock data so you can iterate on UI and wiring before adding Azure SDK integration.

## Run

```bash
go run ./cmd/storage-tui
```

## Controls

- q: quit
- r: refresh data
- tab: cycle focus between accounts, contents, and preview
- enter/right arrow: expand or collapse account or container
- space: toggle subscription selection
- /: search within preview
- esc: clear preview search

## Layout

- `cmd/storage-tui/main.go`: entry point
- `internal/app/app.go`: TUI layout, navigation, and selection details
- `internal/azure/provider.go`: provider interface and mock data
