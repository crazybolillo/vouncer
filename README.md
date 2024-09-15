# vouncer
This is an ARI application designed to assert the calling party has permission to place its call and perform said
call. It depends on an external service to verify if the party should make the call or not, this service is usually
[eryth](https://github.com/crazybolillo/eryth).

## Usage
It is available as a binary or a docker container and is solely configured with environment variables:

### `AST_HOST`
The Asterisk server that the WS connection and API requests will be made to. It should not include the schema.
`http` and `ws` are the only supported schemes at the moment. For example `asterisk.local:8088` or `192.168.1.50:8088`.

### `SERVICE_HOST`
The host that all calls requests will be forwarded to. This service will state whether the call is allowed
or not. As with `ARI_HOST` it must also include the scheme.

### `APP_NAME`
This app name will need to be stated in Asterisk's dialplan to pass call control to the vouncer. Defaults to `vouncer`.

### `CREDENTIALS`
The username and password to authenticate with ARI. It must be in the format `username:password`.
