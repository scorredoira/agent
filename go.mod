module github.com/santiagocorredoira/agent

go 1.24.4

require (
	golang.org/x/term v0.33.0
	textSearch v0.0.0-00010101000000-000000000000
)

replace textSearch => github.com/scorredoira/textSearch v0.0.0-20250726160725-f2cb17ee03e1

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/sys v0.34.0 // indirect
)
