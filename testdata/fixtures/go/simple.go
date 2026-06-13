package fixtures

import (
	"errors"
	"fmt"
)

// UserService handles user operations.
type UserService struct {
	name string
}

// UserRepository defines the data access contract.
type UserRepository interface {
	FindByID(id int) (string, error)
}

// NewUserService creates a new UserService.
func NewUserService(name string) *UserService {
	return &UserService{name: name}
}

// GetName returns the service name.
func (s *UserService) GetName() string {
	fmt.Printf("name: %s\n", s.name)
	return s.name
}

func helperFunc() error {
	return errors.New("not implemented")
}
