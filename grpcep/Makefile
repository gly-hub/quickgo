proto:
	protoc -I. -I${GOPATH}/src -I ../../ -I ./ \
	  --go_out=. --go_opt=paths=source_relative \
	  --go-grpc_out=. --go-grpc_opt=paths=source_relative *.proto
