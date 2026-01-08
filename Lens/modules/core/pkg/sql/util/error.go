// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dal

import (
	"errors"
	errors2 "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"gorm.io/gorm"
)

func CheckErr(err error, allowNotExist bool) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) && allowNotExist {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) && !allowNotExist {
		return err
	}
	return errors2.NewError().WithError(err).WithCode(errors2.CodeDatabaseError)
}
