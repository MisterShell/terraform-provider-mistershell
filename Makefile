default: build

build:
	go build -o terraform-provider-mistershell

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64
	cp terraform-provider-mistershell ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64/

test:
	go test ./... -v

fmt:
	go fmt ./...

clean:
	rm -f terraform-provider-mistershell

.PHONY: default build install test fmt clean
