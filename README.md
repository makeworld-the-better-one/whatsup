# whatsup

whatsup is the reference server implementation for the [fmrl](https://github.com/makeworld-the-better-one/fmrl) protocol.

Currently whatsup has no web interface, but may gain one in the future. For now, the server sysadmin can add users by adding their username and password hash to the config file.

whatsup supports v0.1.1 of the fmrl spec.

## Install

whatsup targets *nix systems, but should work fine on all OSes. You can download binaries from the releases page, or compile from source.

### From source

**Requirements:**
- Go 1.16 or later
- GNU Make

Please note the Makefile does not intend to support Windows, and so there may be issues.

```shell
git clone https://github.com/makeworld-the-better-one/whatsup
cd whatsup
# git checkout v1.2.3 # Optionally pin to a specific version instead of the latest commit
make # Might be gmake on macOS
sudo make install # If you want to install the binary for all users
```

Because you installed with the Makefile, running `whatsup -version` will tell you exactly what commit the binary was built from.

## Deploy

Check out the [example-config.toml](./example-config.toml) file to create your own config, and [whatsup.service](./whatsup.service) for deploying under systemd. Also see `whatsup -help`.


## License

MIT.
