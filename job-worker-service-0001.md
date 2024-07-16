--- 
authors: Dustin Currie (dustinevancurrie@gmail.com)
state: draft
---

# RFD 1: Job Runner Service

## What

Jogger is a CLI and Server that allow users to run jobs on a remote servers--where the Jogger Server is deployed. Users can start, stop, check the status of their jobs, as well as examine the output for jobs that are finished or running.   

## Details

### CLI
`jog`

The CLI has 4 subcommands:
1. `start` -- sends a job to the server, output: `job_id`  
2. `stop` -- sends a stop signal to the server for a given `job_id`, output: `status`
3. `status` -- get the status of a job given the `job_id` output: `status`
4. `output` -- given the `job_id` streams the output of a job, completed or running 

#### UX 
Our users have likely run jobs on remote servers via ssh. The Jogger CLI is different in a key way--it's not interactive. To make the CLI more familiar to these users, the goal is to reduce the CLI's footprint so that users can focus on the commands/jobs they run. The following design decisions stem from this thinking:

#### Jobs are Just Command Strings. 
The number of options in a shell is so large that jogger makes no attempt at structuring them. This reduces the number of flags and arguments that need to be typed to use the CLI. The downside however is that shell substitution happens on the client. There are ways to mitigate this, but for this project they are out of scope. 

#### Use Environment Variables For Connection Information
The following environment variables are supported. These will also be prominently displayed in the USAGE info
```bash
export JOGGER_HOST=<host:port>
export JOGGER_CA_CERT_FILE=<path-to-ca-cert>
export JOGGER_USER_CERT_FILE=<path-to-user-cert>
export JOGGER_USER_PRIVATE_KEY=<path-to-user-private-key>
```

#### Allow Users To Set JobIDs, Make Human Readable IDs by Default
The `-n --name` flag is also available if users would like to specify the `job_id` when starting a job. When duplicates are encountered, a number will be added to the end. By default the `job_id` will be a slug that includes the name of the first command.    

#### Note on CLI Security Configuration
Each use of `jog` creates a new connection with mTLS. For this to work, the user private key file and certificate files should be listed in the environment variables above.

#### Example Usage
```bash
jog start echo 'run the job'
jog start --name=foo echo 'run the job'
jog stop --host=localhost:7654 foo
jog status foo
jog output foo
```

### Security
The Jogger CLI and Server use mTLS to secure communication. For this project, we assume that some future outside service will be responsible for generating and distributing the necessary certificates and keys. For now, certs can be generated manually using `make gen-certs`.   

#### Notes on Keys and Certificates 
For TLS keys, this project uses the ECDSA algorithm with the P256 curve. Keys are generated for the CA, the Jogger Server, and the User who has access to the CLI.
```Go
privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
```
Three certificates are then generated:
1. _Self-Signed CA Certificate_: This certificate is used by both the Client and the Server to verify the identity of the other party. 
2. _Server Certificate_: The Jogger Service uses this certificate to prove its identity to the CLI.
3. _User Certificate_: The User Certificate has the username in the Common Name (CN)) field found here: `Userx509Certificate.Subject.CommonName` see: Access Control

The file locations of these certificates are then added to the user or server environment vars, and read when a connection with mTLS is made.  

### Access Control 
Internally, the Jogger Server ensures that each user only has access to their jobs. To do this, usernames are included in Common Name field found in User Certificates. It is assumed that an external auth system will add these usernames to certs, and ensure usernames are unique. Currently, `make gen-certs` sets the Common Name to `$USER` in the generated User Certificate. If needed this can be modified like so `make USER=lucy gen-certs` To create multiple users the underlying gen-certs code should be edited.   

When a request is made, the User Certificate can be retrieved from the context using
```go
import "google.golang.org/grpc/peer"
...
p, ok := peer.FromContext(ctx)
```
The username then becomes a parameter in calls to the internal job manager

### Server
The server implements a GRPC API with a service method for each of the CLI subcommands see: [job_service.proto](https://github.com/dustinevan/jogger/blob/develop/pkg/proto/job_service.proto)

`make grpc` generates the protobuf, grpc client, and grpc server stubs. `buf` is used internally.

after reading the username from the peer certificate the internal Job Manager API is called


### Job Manager API
The Job Manager API holds references to all jobs run since the server was started. Storing job data is out of scope, so everything is held in memory.

#### Starting A Job
- cgroups-v2
  - If this is a new user, add a new entry in the tree
  - Once PID is available add it to user cgroup node
- Set stdout and stderr buffers
- Start `exec.Cmd` in a go routine -- include panic recover and cancellation
- Set `exec.Cmd` cancel function to terminate first, then kill if needed
- Implement a max concurrent jobs limit of 100
- The job has started when the API returns without error.
- Produce a single cancel function for each job that returns the end status

#### Stopping A Job
- If the client cancels the context before the Start command returns, it should cancel the job.
- Stop function calls the job's cancel function and returns the status

#### Getting the Status 
- Look up the job by username and id, then return the status

#### Streaming Output
- Each command is set up with an externally defined buffer for stderr and stdout. 
- When an output request is received, a read only channel of output data is provided. 
- This channel is written to by a separate goroutine which is reading the stdout and stderr buffers 
  - It can be canceled when the client closes the context
  - It loops through the buffer and writes at most 64k chunks to the channel
  - Writes happen within at least a second of it finding new data in the buffer
  - The loop ends when the job enters one of the `done` states and all the data is read. 

### Concurrency
It's likely that mutexes will be needed to protect the internal state of the Job Manager. The Job Manager will be the only place where mutexes are used.

### Context, Cancellation, and Graceful Shutdown
- When the server is shutdown, the server needs to be stopped, then all running jobs need to be canceled. 
- If the server receives a context cancellation from the client before it returns, started jobs should be stopped, output streams should be stopped, and stop calls should be canceled
 if signals haven't been sent yet. 
- The client is responsible for checking the status after canceling a stop request to see whether the job is still running or not. 

#### cgroups-v2
cgroups can be manually managed through the cgroup file system found at `/sys/fs/cgroup` in the project's Docker image. The cgroupfs controls resource allocation according to the 
directory structure. By making new directories in the structure and adding PIDs to those directories, sibling directories are given equal resources by default.

The Jogger cgroup implementation does the following:
- When the Jogger server starts, a new cgroup is made for it: `mkdir /sys/fs/cgroup/jogger/`
- The cpu, memory, and io controller are added for the jogger cgroup and its children
- When a new user is encountered a new sub-level is created for it `mkdir /sys/fs/cgroup/jogger/<username>`
- When a job is started, the PID is added to the user's `cgroup.procs` file

Note
: The Jogger Server must run with root permissions within the Docker container to manage cgroupfs. 
: Because the Jogger Server is a parent of all the user cgroups its resources are not limited by the cgroupfs. This is important because the server needs to be able to manage all the user cgroups without being crowded out by users. It could be that resources need to be explicitly set aside for the server, but this is out of scope for this project.
 

### Docker
- cgroups-v2 is a linux feature, for testing, the project includes a Ubuntu 24.04 image with Go 1.22.5. This dockerfile must be improved before production use.

