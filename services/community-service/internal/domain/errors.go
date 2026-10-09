package domain

import "errors"

var (
	ErrNotFound          = errors.New("resource not found")
	ErrEmptyCart         = errors.New("cart is empty")
	ErrInsufficientStock = errors.New("insufficient product inventory")
	ErrInvalidCategory   = errors.New("forum category does not exist")
)
