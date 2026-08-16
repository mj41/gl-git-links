.PHONY: all regen-exa build test clean

all: regen-exa build test

build:
	go build -o git-glfix ./cmd/git-glfix/
	go build -o git-glfix-test ./cmd/git-glfix-test/

# Both suites. The incremental-cache tests were written, committed and then
# never run by anything: --test-cache existed only as a flag someone had to know
# about. They pass, and they cover the snapshot logic nothing else touches.
test: build
	./git-glfix-test --repo ../gl-exA --tool ./git-glfix
	./git-glfix-test --repo ../gl-exA --tool ./git-glfix --test-cache

regen-exa:
	rm -rf ../gl-exA
	cd ../git-rgen-tool && go run ./cmd/rgen --conf ../gl-exA-src/rgen-conf.json

clean:
	rm -f git-glfix git-glfix-test
