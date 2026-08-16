.PHONY: all regen-exa build test clean

all: regen-exa build test

build:
	go build -o git-glfix ./cmd/git-glfix/
	go build -o git-glfix-test ./cmd/git-glfix-test/

test: build
	./git-glfix-test --repo ../gl-exA --tool ./git-glfix

regen-exa:
	rm -rf ../gl-exA
	cd ../git-rgen-tool && go run ./cmd/rgen --conf ../gl-exA-src/rgen-conf.json

clean:
	rm -f git-glfix git-glfix-test
