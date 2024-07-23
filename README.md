# jogger

A simple remote job runner.

### Quickstart Guide -- Local Development and Testing

Currently, Jogger should be built from source. A `Makefile` is provided to simplify setup. 

```bash
# 1. Clone the repository:
git clone git@github.com:dustinevan/jogger.git

# 2.Change to the repository directory:
cd jogger

# 3. Jogger uses gRPC over mTLS to communicate between client and server. 
# Generate the keys and certificates for local development:
make gen-certs

# 4. gen-certs prints the environment export commands needed to run the
# server and client. These environment variables point to the keys and
# certificates needed for mTLS. Run the exports:
...

# 5. Install the client binary: `jog`, and check that it is in the PATH:
make install-cli
which jog

# 6. Run the server -- note that currently the server must run on localhost:50051
# as this address is encoded in the server's certificate:
make run-server

# 7. Start your first job:
jog run -- echo "Hello, World!"

# 8. Checkout what jog can do:
jog --help
```


