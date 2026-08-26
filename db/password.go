package db

import (
	"math/rand/v2"
	"unicode"

	"go.rtnl.ai/x/randstr"
)

const (
	lowercase      = "abcdefghijklmnopqrstuvwxyz"
	uppercase      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits         = "0123456789"
	specials       = "-._~"
	alphabet       = lowercase + uppercase + digits + specials
	passwordLength = 32
)

// Generates a strong password that meets the requirements of the database.
func Password() string {
	pw := randstr.Generate(passwordLength, alphabet)
	if !IsStrongPassword(pw) {
		return randReplace(pw, lowercase, uppercase, digits, specials)
	}
	return pw
}

func IsStrongPassword(password string) bool {
	if len(password) < passwordLength {
		return false
	}

	var (
		hasUpper, hasLower, hasDigit, hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasDigit && hasSpecial
}

// Replaces a random character in the password with a random character from the
// replacement string. By using permutations, this function ensures that a replaced
// character is not re-replaced with different character, ensuring that at least one
// character from each replacement set will be presentin the final password.
func randReplace(password string, replacements ...string) string {
	runes := []rune(password)
	indexes := rand.Perm(len(password))
	for i, replacement := range replacements {
		if i >= len(indexes) {
			break
		}
		runes[indexes[i]] = rune(replacement[rand.IntN(len(replacement))])
	}
	return string(runes)
}
