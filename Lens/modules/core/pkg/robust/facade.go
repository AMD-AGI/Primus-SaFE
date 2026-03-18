// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package robust provides the Robust API HTTP client.
// The RobustFacade (which depends on database.Facade) is assembled in
// the server package to avoid import cycles: robust → database → clientsets → robust.
// See server/robust_facade.go for the actual facade wiring.
package robust
