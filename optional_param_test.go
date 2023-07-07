package main

import (
	"errors"
	"fmt"
	"testing"
)

type Config struct {
	Name  string
	Value int
}

type ConfigFunc func(*Config) error

func WithName(name string) ConfigFunc {
	return func(cfg *Config) error {
		if cfg == nil {
			return errors.New("nil pointer")
		}
		cfg.Name = name
		return nil
	}
}

func WithValue(value int) ConfigFunc {
	return func(cfg *Config) error {
		if cfg == nil {
			return errors.New("nil pointer")
		}
		if value < 0 {
			return errors.New("invalid value")
		}
		cfg.Value = value
		return nil
	}
}

func ApplyConfig(cfg *Config, funcs ...ConfigFunc) error {
	for _, f := range funcs {
		err := f(cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestPointerStruct(t *testing.T) {
	// Contoh 1: Nil Pointer
	var cfg *Config
	err := ApplyConfig(cfg, WithName("John"))
	if err != nil {
		fmt.Println("Error:", err)
	}

	// Contoh 2: Penggunaan Nilai Default
	cfg2 := &Config{Name: "Alice", Value: -1}
	err = ApplyConfig(cfg2)
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Config:", cfg2)

	// Contoh 3: Aliasing dan Kesalahan Mutasi
	cfg3 := &Config{Name: "Bob", Value: 10}
	funcs := []ConfigFunc{
		WithName("Charlie"),
		WithValue(20),
	}
	err = ApplyConfig(cfg3, funcs...)
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Config:", cfg3)

	// Contoh 4: Ketidaksesuaian Tipe
	cfg4 := &Config{Name: "Dave", Value: 30}
	err = ApplyConfig(cfg4, WithName("42"))
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Config:", cfg4)

	// Contoh 5: Kesalahan Logika
	cfg5 := &Config{Name: "Eve", Value: 40}
	err = ApplyConfig(cfg5, WithName("Eve"), WithValue(-1))
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Config:", cfg5)
}
