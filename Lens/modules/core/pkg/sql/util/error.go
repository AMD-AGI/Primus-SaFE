package dal

import (
	"errors"
	errors2 "github.com/AMD-AGI/primus-lens/core/pkg/errors"
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
