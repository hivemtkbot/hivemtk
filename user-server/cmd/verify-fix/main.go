package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"marketing/internal/service"
)

func main() {
	s := service.SendMessageRequest{}
	t := reflect.TypeOf(s)

	v := validator.New()

	// Test 1: empty struct
	err := v.Struct(s)
	fmt.Printf("Empty struct validation: %v\n", err)

	// Test 2: with content only
	s2 := service.SendMessageRequest{Content: "test"}
	err = v.Struct(s2)
	fmt.Printf("Only content: %v\n", err)

	// Test 3: with sender_type=agent
	s3 := service.SendMessageRequest{Content: "test", SenderType: "agent"}
	err = v.Struct(s3)
	fmt.Printf("Content + sender_type=agent: %v\n", err)

	// Check the field tags
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if strings.Contains(f.Name, "Sender") {
			fmt.Printf("Field %s: tag=%q\n", f.Name, f.Tag)
		}
	}
}
