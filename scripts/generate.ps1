# PowerShell equivalent of `make generate` for Windows hosts without Make.
$ErrorActionPreference = 'Stop'

$env:Path = "$env:LOCALAPPDATA\protoc\bin;$env:USERPROFILE\go\bin;$env:Path"

New-Item -ItemType Directory -Force -Path internal/proto, api/swagger | Out-Null

protoc `
  --proto_path=api/proto `
  --proto_path=third_party/googleapis `
  --proto_path=third_party `
  --go_out=internal/proto --go_opt=paths=source_relative `
  --go-grpc_out=internal/proto --go-grpc_opt=paths=source_relative `
  --openapiv2_out=api/swagger `
  --openapiv2_opt=allow_merge=true,merge_file_name=gophkeeper,json_names_for_fields=false `
  api/proto/common.proto `
  api/proto/auth.proto `
  api/proto/data.proto `
  api/proto/sync.proto

go run ./scripts/swagger2yaml.go -in api/swagger/gophkeeper.swagger.json -out api/swagger.yaml

Write-Host 'Generated internal/proto and api/swagger.yaml'
