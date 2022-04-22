module github.com/twmb/kcl

go 1.15

require (
	github.com/BurntSushi/toml v0.4.1
	github.com/aws/aws-sdk-go v1.40.55
	github.com/spf13/cobra v1.2.1
	github.com/twmb/franz-go v1.2.3
	github.com/twmb/franz-go/pkg/kadm v0.0.0-20220331035613-01d0c45d69d2
	github.com/twmb/franz-go/pkg/kmsg v0.0.0-20211104051938-70808186d5f7
	github.com/twmb/go-strftime v0.0.0-20190915101236-e74f7c4fe4fa
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
)

replace github.com/twmb/kcl v0.8.1-0.20220414160959-ca658f9725f8 => github.com/Zach-Johnson/kcl v0.8.1-0.20220422010454-12f4e09528cc
