module github.com/therealmcsparrow/mcharbor-addons/plugins/storage-samba

go 1.27.2

require (
	github.com/hirochachacha/go-smb2 v1.1.0
	github.com/therealmcsparrow/mcharbor v0.0.0
)

require (
	github.com/geoffgarside/ber v1.2.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
)

replace github.com/therealmcsparrow/mcharbor => ../../../backend
