.PHONY: start build

NOW = $(shell date -u '+%Y%m%d%I%M%S')

APP = $(shell cat vname)

dev: main

# 初始化mod
init:
	go mod init ${APP}

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o ./_out/$(APP) ./

# go env -w GOPROXY=https://proxy.golang.com.cn,direct
proxy:
	go env -w GO111MODULE=on
	go env -w GOPROXY=http://mvn.res.local/repository/go,direct
	go env -w GOSUMDB=sum.golang.google.cn

helm:
	helm -n default template deploy/chart > deploy/bundle.yml

# -eng rdx/mux/map
main:
	go run main.go -eng map -local -dual -c __zgg.toml
# -tpl ./tmpl
tenv:
	ZGG_SERVER_PORT=81 go run main.go -local -debug -print

test:
	_out/$(APP) version

hello:
	go run main.go hello

cert:
	go run main.go cert -domain localhost -path _out/cert

# https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-v1.19.2-linux-amd64.tar.gz
test-kube:
	TEST_ASSET_ETCD=_out/kubebuilder/bin/etcd \
	TEST_ASSET_KUBE_APISERVER=_out/kubebuilder/bin/kube-apiserver \
	TEST_ASSET_KUBECTL=_out/kubebuilder/bin/kubectl \
	go test -v -run TestCustom testdata/custom_test.go

test-custom:
	go test -v cmd/custom_test.go

test-cert:
	go test -v ze/crt/cert_test.go -run Test_cert

push:
	git push --set-upstream origin $b

tflow:
	@if [ -z "$(tag)" ]; then \
		echo "error: 'tag' not specified! Please specify the 'tag' using 'make tflow tag=(version)-(appname)'";\
		exit 1; \
	fi
	git commit -am "${tag}" && git tag -a $(tag) -m "${tag}" && git push origin $(tag) && git reset --hard HEAD~1

git:
	@if [ "$(m)" ]; then \
		git add -A && git commit -am "$(m)" && git push; \
	fi
	@if [ "$(t)" ]; then \
	 	git tag -a $(t) -m "${t}" && git push origin $(t); \
	fi