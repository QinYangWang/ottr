// Package web embeds the frontend of the app.
package web

import "embed"

//go:embed index.html style.css app.js
var FS embed.FS
