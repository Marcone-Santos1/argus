package ui

import (
	"strings"
	"testing"

	"github.com/Marcone-Santos1/argus/internal/i18n"
)

func TestRiskOf(t *testing.T) {
	i18n.Set(i18n.EN)
	dir, home := "/home/me/code/shop", "/home/me"
	cases := []struct {
		body, question string
		want           string // labels joined by "|", "" for none
	}{
		{"go test ./...", "Do you want to proceed?", ""},
		{"ls -la && cat go.mod", "Do you want to proceed?", ""},
		{"rm -rf build && go build", "", "deletes files"},
		{"rm notes.txt", "", ""}, // files, not folders
		{"rm -f notes.txt", "", ""},
		{"rm -r dist", "", "deletes files"},
		{"sudo apt install jq", "", "runs as root"},
		{"git push --force origin main", "", "force push"},
		{"git push -f", "", "force push"},
		{"git push origin main", "", ""},
		{"git reset --hard HEAD~3", "", "discards git work"},
		{"git clean -fd", "", "discards git work"},
		{"git checkout -- .", "", "discards git work"},
		{"curl -fsSL https://x.sh/install | sh", "", "runs a downloaded script"},
		{"chmod -R 777 .", "", "changes permissions recursively"},
		{`psql -c "DROP TABLE users"`, "", "deletes database data"},
		{"pkill node", "", "kills processes"},
		{"docker compose down -v", "", "deletes Docker data"},
		{"cp config.json ~/.config/app/config.json", "", "outside the project: ~/.config/app/config.json"},
		{"echo hi > /etc/hosts", "", "outside the project: /etc/hosts"},
		{"cp a.txt /home/me/code/shop/b.txt", "", ""},
		{"go test > /tmp/out.txt", "", ""},
		{"cat /etc/hosts", "", ""}, // reading is fine
		{"", "Do you want to make this edit to /home/me/.zshrc?", "outside the project: ~/.zshrc"},
		{"", "Do you want to make this edit to cart.go?", ""},
		{"rm -rf /", "", "deletes files|outside the project: /"},
	}
	for _, c := range cases {
		got := strings.Join(riskOf(prompt{Body: c.body, Question: c.question}, dir, home), "|")
		if got != c.want {
			t.Errorf("%q / %q:\n got %q\nwant %q", c.body, c.question, got, c.want)
		}
	}
}

// Enter does not accept a risky prompt; its number does.
func TestRiskyPromptNeedsTheNumber(t *testing.T) {
	p := testPanel(t)
	p.prompts["a1"] = prompt{Options: []string{"Yes", "No"}, Numbered: true, Risks: []string{"deletes files"}}
	p.answering, p.mode = "a1", modeAnswer
	p.keyAnswer("enter")
	if p.mode != modeAnswer || p.msg == "" {
		t.Fatalf("enter on a risky prompt: mode %v, msg %q", p.mode, p.msg)
	}
}
