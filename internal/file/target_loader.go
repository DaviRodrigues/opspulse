package file

import (
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type Loader interface {
	Load() ([]domain.Target, error)
}
