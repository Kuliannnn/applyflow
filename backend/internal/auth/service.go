package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid credentials input")
	ErrCredentials = errors.New("invalid email or password")
	ErrEmailTaken  = errors.New("email already registered")
	ErrNotFound    = errors.New("identity not found")
)

type Identity struct {
	ID, Email      string
	ProfileVersion int64
	CreatedAt      time.Time
}
type Account struct {
	Identity
	PasswordHash string
}
type Store interface {
	Create(context.Context, string, string) (Identity, error)
	ByEmail(context.Context, string) (Account, error)
	ByID(context.Context, string) (Identity, error)
}
type Passwords interface {
	Hash(string) (string, error)
	Compare(string, string) bool
}
type Service struct {
	store     Store
	passwords Passwords
	dummyHash string
}

func New(store Store, passwords Passwords) (*Service, error) {
	dummy, err := passwords.Hash("dummy-password-for-timing-only")
	if err != nil {
		return nil, err
	}
	return &Service{store: store, passwords: passwords, dummyHash: dummy}, nil
}
func normalize(email, password string, register bool) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, err := mail.ParseAddress(email)
	min := 1
	if register {
		min = 12
	}
	if err != nil || a.Address != email || len(email) > 254 || !utf8.ValidString(password) || len(password) > 72 || utf8.RuneCountInString(password) < min {
		return "", ErrInvalid
	}
	return email, nil
}
func (s *Service) Register(ctx context.Context, email, password string) (Identity, error) {
	email, err := normalize(email, password, true)
	if err != nil {
		return Identity{}, err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return Identity{}, err
	}
	return s.store.Create(ctx, email, hash)
}
func (s *Service) Login(ctx context.Context, email, password string) (Identity, error) {
	email, err := normalize(email, password, false)
	if err != nil {
		return Identity{}, err
	}
	account, err := s.store.ByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		s.passwords.Compare(s.dummyHash, password)
		return Identity{}, ErrCredentials
	}
	if err != nil {
		return Identity{}, err
	}
	if !s.passwords.Compare(account.PasswordHash, password) {
		return Identity{}, ErrCredentials
	}
	return account.Identity, nil
}
func (s *Service) Identity(ctx context.Context, id string) (Identity, error) {
	return s.store.ByID(ctx, id)
}
