package utils

import "testing"

func TestContainsKeyword(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		keywords string
		expected bool
	}{
		{"single keyword match", "Hello world", "Hello", true},
		{"multiple keywords match", "Hello world", "Hi,Hello,Greetings", true},
		{"no keyword match", "Hello world", "Goodbye,Bye", false},
		{"empty keywords", "Hello world", "", true}, // 空关键词列表时,Split 返回 [""],Contains("","") 为 true
		{"empty message", "", "Hello", false},
		{"case sensitive", "Hello World", "hello", false},
		{"partial match", "Say Hello to everyone", "Hello", true},
		{"keyword with spaces in list", "Hello world", "Hi, Hello ,Greetings", true},
		{"multiple keywords with spaces", "Hello world", "Hi,Hello,Greetings", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsKeyword(tt.message, tt.keywords)
			if result != tt.expected {
				t.Errorf("ContainsKeyword(%q, %q) = %v, expected %v", tt.message, tt.keywords, result, tt.expected)
			}
		})
	}
}

func TestContainsKeyword_CommaSeparated(t *testing.T) {
	message := "I want to buy a car"
	keywords := "bike,train,car,bus"

	result := ContainsKeyword(message, keywords)
	if !result {
		t.Errorf("Expected ContainsKeyword to return true for keyword 'car'")
	}
}

func TestContainsKeyword_NoMatch(t *testing.T) {
	message := "I like apples"
	keywords := "orange,banana,grape"

	result := ContainsKeyword(message, keywords)
	if result {
		t.Errorf("Expected ContainsKeyword to return false")
	}
}
