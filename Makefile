BIN ?= $(HOME)/.local/bin/argus

.PHONY: build install test race cover vet fmt check bench

build: ## compila em ./bin/argus
	go build -o bin/argus ./cmd/argus

install: ## compila em ~/.local/bin/argus e roda `argus install`
	go build -o $(BIN) ./cmd/argus
	$(BIN) install

test: ## testes
	go test ./...

race: ## testes com o detector de corrida
	go test -race -count=1 ./...

cover: ## cobertura por pacote
	go test -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check: vet ## o que o CI roda
	@test -z "$$(gofmt -l .)" || (echo "rode make fmt:"; gofmt -l .; exit 1)
	go test -race -count=1 ./...

bench: ## mede o desempenho com o painel aberto
	$(BIN) bench
