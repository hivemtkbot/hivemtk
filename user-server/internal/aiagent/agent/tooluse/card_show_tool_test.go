package tooluse

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
)

func TestCardShowTool_Execute(t *testing.T) {
	tool := NewCardShowTool()
	if tool.Name() != "card.show" {
		t.Fatalf("unexpected tool name: %s", tool.Name())
	}
	if tool.Category() != CategoryCard {
		t.Fatalf("unexpected category: %s", tool.Category())
	}

	args := map[string]any{
		"type":        "product",
		"title":       "iPhone 15",
		"subtitle":    "256G 蓝色",
		"description": "A17 芯片，续航提升",
		"image_url":   "https://example.com/ip15.png",
		"fields": map[string]any{
			"价格": "5999",
			"规格": "256G",
			"库存": "现货",
		},
		"buttons": []any{
			map[string]any{"text": "立即购买", "url": "https://shop.example.com/buy/15", "action": "buy"},
		},
	}

	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if res.Card == nil {
		t.Fatal("expected card attached to tool result")
	}
	card := res.Card
	if card.Type != model.CardTypeProduct || card.Title != "iPhone 15" {
		t.Fatalf("unexpected card: %+v", card)
	}
	if card.Subtitle != "256G 蓝色" || card.Description != "A17 芯片，续航提升" {
		t.Fatalf("unexpected card text: %+v", card)
	}
	if len(card.Buttons) != 1 || card.Buttons[0].Text != "立即购买" || card.Buttons[0].URL != "https://shop.example.com/buy/15" {
		t.Fatalf("unexpected buttons: %+v", card.Buttons)
	}
	if card.Fields["价格"] != "5999" || card.Fields["规格"] != "256G" {
		t.Fatalf("unexpected fields: %+v", card.Fields)
	}
}

func TestCardShowTool_RequiresTitle(t *testing.T) {
	tool := NewCardShowTool()
	_, err := tool.Execute(context.Background(), map[string]any{"type": "generic"})
	if err == nil {
		t.Fatal("expected error when title is missing")
	}
}

func TestCardShowTool_InvalidType(t *testing.T) {
	tool := NewCardShowTool()
	_, err := tool.Execute(context.Background(), map[string]any{"title": "x", "type": "banana"})
	if err == nil {
		t.Fatal("expected error for invalid card type")
	}
}

func TestCardShowTool_DefaultsToGeneric(t *testing.T) {
	tool := NewCardShowTool()
	res, err := tool.Execute(context.Background(), map[string]any{"title": "仅标题"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Card == nil || res.Card.Type != model.CardTypeGeneric {
		t.Fatalf("expected generic card, got %+v", res.Card)
	}
}
