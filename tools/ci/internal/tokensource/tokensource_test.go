package tokensource_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ci-tools/internal/tokensource"
)

func TestStatic_FetchCredential(t *testing.T) {
	got, err := tokensource.Static("static-credential").FetchCredential(context.Background())
	if err != nil {
		t.Fatalf("FetchCredential() error = %v", err)
	}
	want := tokensource.Credential{
		Value: "static-credential",
		Kind:  tokensource.KindAPIKey,
	}
	if got != want {
		t.Errorf("FetchCredential() = %+v, want %+v", got, want)
	}

	_, err = tokensource.Static("").FetchCredential(context.Background())
	if !errors.Is(err, tokensource.ErrEmptyStatic) {
		t.Errorf("FetchCredential() error = %v, want ErrEmptyStatic", err)
	}
}

func TestStatic_Token(t *testing.T) {
	got, err := tokensource.Static("static-credential").Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != "static-credential" {
		t.Errorf("Token() = %q, want %q", got, "static-credential")
	}

	_, err = tokensource.Static("").Token(context.Background())
	if !errors.Is(err, tokensource.ErrEmptyStatic) {
		t.Errorf("Token() error = %v, want ErrEmptyStatic", err)
	}
}

func TestNewVaultKV_Delegation(t *testing.T) {
	_, err := tokensource.NewVaultKV(tokensource.VaultKVConfig{})
	if err == nil {
		t.Fatal("NewVaultKV() with empty config expected validation error, got nil")
	}

	valid := tokensource.VaultKVConfig{
		VaultAddr:   "https://vault.example.com",
		Role:        "ci-role",
		JWT:         "jwt-token",
		MountPath:   "secret",
		SecretPath:  "ci/credentials",
		SecretField: "api_key",
	}
	vk, err := tokensource.NewVaultKV(valid)
	if err != nil {
		t.Fatalf("NewVaultKV() unexpected error = %v", err)
	}
	if vk == nil {
		t.Fatal("NewVaultKV() returned nil instance")
	}
}

func TestStatic_ConcurrentCallsReturnTheSameCredential(t *testing.T) {
	const callers = 128

	source := tokensource.Static("shared-credential")
	results := make([]tokensource.Credential, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = source.FetchCredential(context.Background())
		}(i)
	}
	wg.Wait()

	want := tokensource.Credential{
		Value: "shared-credential",
		Kind:  tokensource.KindAPIKey,
	}
	for i := range callers {
		if errs[i] != nil || results[i] != want {
			t.Errorf("caller %d: FetchCredential() = %+v, %v", i, results[i], errs[i])
		}
	}
}
