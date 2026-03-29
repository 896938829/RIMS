// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package main

import (
	"log"

	"rims-go/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
