package dictionaries

import "context"

type Repository interface {
	ListEgressCountries(ctx context.Context) ([]string, error)
}
