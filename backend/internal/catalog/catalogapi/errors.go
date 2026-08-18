package catalogapi

import "errors"

var (
	ErrProductNotFound       = errors.New("catalogapi: product not found")
	ErrMaterialNotFound      = errors.New("catalogapi: material not found")
	ErrPricingNotAvailable   = errors.New("catalogapi: pricing not available for this product+material combination")
	ErrDimensionsOutOfRange  = errors.New("catalogapi: dimensions out of product's min/max range")
	ErrDimensionsInvalid     = errors.New("catalogapi: dimensions must be positive integers")
	ErrPricingTypeMismatch   = errors.New("catalogapi: pricing row does not match parent product's pricing_type")
)
