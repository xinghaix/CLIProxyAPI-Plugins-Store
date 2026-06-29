package main

import "embed"

//go:embed web-dist/*
var embeddedWebFS embed.FS
