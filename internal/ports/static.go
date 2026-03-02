package ports

import "embed"

//go:embed ui.html swagger.html
var uiFiles embed.FS
