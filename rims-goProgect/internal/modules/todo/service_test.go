package todo

import "testing"

func TestServiceCreateValidatesTitle(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.Create(CreateTodoInput{Title: ""}); err == nil {
		t.Fatalf("expected error when title is empty")
	}
}
