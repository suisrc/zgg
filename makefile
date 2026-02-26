.PHONY: start build

NOW = $(shell date -u '+%Y%m%d%I%M%S')

APP = $(shell cat app/vname)

dev: main

# 初始化mod
init:
	go mod init ${APP}

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build -ldflags "-w" -o ./_out/$(APP) ./app

# go env -w GOPROXY=https://proxy.golang.com.cn,direct
proxy:
	go env -w GO111MODULE=on
	go env -w GOPROXY=http://mvn.res.local/repository/go,direct
	go env -w GOSUMDB=sum.golang.google.cn

helm:
	helm -n default template deploy/chart > deploy/bundle.yml

# -eng rdx/mux/map
main:
	go run app/main.go -eng map -local -dual -c __zgg.toml
# -tpl ./tmpl
tenv:
	ZGG_FRONT2_INDEXS_0="/api/v1/=http://127.0.0.1" ZGG_FRONT2_TPROOT="none"  go run app/main.go -local -debug -print -port 81

test:
	_out/$(APP) -local -debug -port 81

hello:
	go run app/main.go hello

cert:
	go run app/main.go cert -domain localhost -path _out/cert

# https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-v1.19.2-linux-amd64.tar.gz
test-kube:
	TEST_ASSET_ETCD=_out/kubebuilder/bin/etcd \
	TEST_ASSET_KUBE_APISERVER=_out/kubebuilder/bin/kube-apiserver \
	TEST_ASSET_KUBECTL=_out/kubebuilder/bin/kubectl \
	go test -v -run TestCustom testdata/custom_test.go

cmd-custom:
	go test -v cmd/custom_test.go

cmd-cert:
	go run app/main.go cert --path _out/cert

cmd-certca:
	go run app/main.go certca --path _out/cert

cmd-certsa:
	go run app/main.go certsa --path _out/cert

cmd-certce:
	go run app/main.go certce --path _out/cert

cmd-cert-exp:
	go run app/main.go cert-exp

push:
	git push --set-upstream origin $b

git:
	@if [ "$(m)" ]; then \
		git add -A && git commit -am "$(m)" && git push; \
	fi
	@if [ "$(t)" ]; then \
	 	git tag -a $(t) -m "${t}" && git push origin $(t); \
	fi

front2:
	@if [ -z "$(tag)" ]; then \
		echo "error: 'tag' not specified! Please specify the 'tag' using 'make front2 tag=(version)";\
		exit 1; \
	fi
	sed -i -e 's|// front2.Init3(os.|front2.Init3(os.|g' -e '7i"os"' -e '7i"github.com/suisrc/zgg/app/front2"' app/main.go
	git commit -am "${tag}" && git tag -a $(tag)-front2 -m "${tag}" && git push origin $(tag)-front2 && git reset --hard HEAD~1

kwlog2:
	@if [ -z "$(tag)" ]; then \
		echo "error: 'tag' not specified! Please specify the 'tag' using 'make kwlog2 tag=(version)";\
		exit 1; \
	fi
	sed -i -e 's|// kwlog2.|kwlog2.|g' -e '7i"github.com/suisrc/zgg/app/kwlog2"' app/main.go
	git commit -am "${tag}" && git tag -a $(tag)-kwlog2 -m "${tag}" && git push origin $(tag)-kwlog2 && git reset --hard HEAD~1

kwdog2:
	@if [ -z "$(tag)" ]; then \
		echo "error: 'tag' not specified! Please specify the 'tag' using 'make kwdog2 tag=(version)";\
		exit 1; \
	fi
	sed -i -e 's|// z.HttpServeDef|z.HttpServeDef|g' \
	-e 's|// proxy2.|proxy2.|g' -e '7i"github.com/suisrc/zgg/app/proxy2"' \
	-e 's|// kwdog2.|kwdog2.|g' -e '7i"github.com/suisrc/zgg/app/kwdog2"' app/main.go
	git commit -am "${tag}" && git tag -a $(tag)-kwdog2 -m "${tag}" && git push origin $(tag)-kwdog2 && git reset --hard HEAD~1

wgetar:
	@if [ -z "$(tag)" ]; then \
		echo "error: 'tag' not specified! Please specify the 'tag' using 'make wgetar tag=(version)";\
		exit 1; \
	fi
	cp wget_tar Dockerfile
	git commit -am "${tag}" && git tag -a $(tag)-wgetar -m "${tag}" && git push origin $(tag)-wgetar && git reset --hard HEAD~1