package main

import "embed"

//go:embed web-dist/* web-dist/assets/*
var embeddedWebFS embed.FS
