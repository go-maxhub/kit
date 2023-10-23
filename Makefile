install_protoc:
	brew install protobuf
	brew install grpc
	go install google.golang.org/grpc@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	sudo apt install protobuf-compiler

protoc:
	protoc proto/*.proto --go_out=. --go-grpc_out=.

clean:
	sudo rm -rf proto/*.go

grpc_cli:
	git clone https://github.com/grpc/grpc
	cd grpc
	make grpc_cli
	cd bins/opt
	./grpc_cli ls localhost:8081

grpc_ls:
	./grpc_cli ls localhost:8081