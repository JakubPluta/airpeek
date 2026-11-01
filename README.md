# airpeek

A terminal UI for looking at Apache Airflow: DAGs, their runs, tasks and
logs. It only reads. There is no way to trigger, pause or clear anything
from it.

```
local  DAGs › example_bash_operator › manual__2026-11-01T18:31:42+00:00   read-only
STATE      TASK               DURATION   TIMELINE 18:31:57 → 18:32:15 UTC
success    runme_0            1s         ██████▎
success    runme_1            1s                    ▐██████▏
success    runme_2            1s                                ▐██████▏
success    also_run_this      0s                                            ▊
skipped    this_will_skip     0s                                                ▐▎
success    run_after_loop     0s                                                       ▐
```

It works with Airflow 2 wherever it runs: Docker on your laptop,
Kubernetes, Astronomer, Google Cloud Composer and Amazon MWAA. Airflow 3
is not supported yet.

## Getting started

```bash
go install github.com/JakubPluta/airpeek/cmd/airpeek@latest   # needs Go 1.24+
airpeek init    # answer a few questions about your Airflow
airpeek         # browse
```

`airpeek init` asks where your Airflow runs and how to log in, then saves
the answers to `~/.config/airpeek/config.yaml`. Run it again to add more
environments; airpeek lets you pick one at start, or opens one directly
with `airpeek --env <name>`.

Passwords and tokens never go into the file. It stores the names of
environment variables, and airpeek reads them when you open that
environment. Run `init` and it tells you which ones to set.

## What init asks for

### Docker or Kubernetes

The address of the Airflow UI and a username and password.

The official
[docker-compose.yaml](https://airflow.apache.org/docs/apache-airflow/stable/howto/docker-compose/index.html)
runs at `http://localhost:8080` with `airflow` / `airflow`. A user who can
only read is better:

```bash
docker compose exec airflow-webserver \
  airflow users create --role Viewer --username viewer --password viewer \
  --firstname View --lastname Only --email viewer@example.com
```

On Kubernetes, use your ingress address or forward the webserver to
`localhost:8080`:

```bash
kubectl port-forward svc/airflow-webserver 8080:8080 -n airflow
```

The API there refuses passwords until you allow it. With the official Helm
chart, add this to your values:

```yaml
config:
  api:
    auth_backends: airflow.api.auth.backend.basic_auth,airflow.api.auth.backend.session
```

### Astronomer

The Airflow API URL from your Deployment's details page, without `/api/v1`
at the end, and a Deployment API token. Use a token with the most limited
role your organization allows.

### Google Cloud Composer

Composer 2 and 3. The address comes from:

```bash
gcloud composer environments describe <environment> --location <region> \
  --format='value(config.airflowUri)'
```

airpeek logs in with your gcloud account, so run
`gcloud auth application-default login` once. The account needs the
Viewer role in Airflow:

```bash
gcloud composer environments run <environment> --location <region> \
  users add-role -- -e <your-email> -r Viewer
```

### Amazon MWAA

An AWS profile. `init` lists the MWAA environments the profile can see and
you pick one. MWAA goes through the AWS API, so it works with a private
webserver and without a VPN.

To keep the profile to reading, allow it only Airflow's Viewer role:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "airflow:ListEnvironments", "Resource": "*" },
    {
      "Effect": "Allow",
      "Action": "airflow:InvokeRestApi",
      "Resource": "arn:aws:airflow:<region>:<account-id>:role/<environment>/Viewer"
    }
  ]
}
```

## The config file

You can also edit it by hand. This is what `init` writes, one entry per
platform:

```yaml
environments:
  - name: local
    http:
      url: http://localhost:8080
      auth: basic
      username_env: AIRFLOW_USERNAME
      password_env: AIRFLOW_PASSWORD

  - name: astro
    http:
      url: https://<organization>.astronomer.run/<deployment>
      auth: token
      token_env: ASTRO_API_TOKEN

  - name: composer
    http:
      url: https://<id>-dot-<region>.composer.googleusercontent.com
      auth: google

  - name: mwaa-prod
    highlight: true        # red header, so you know where you are
    mwaa:
      profile: prod
      region: eu-north-1    # optional, defaults to the profile's
      environment: my-environment
```

Use `--config` or `AIRPEEK_CONFIG` to keep the file somewhere else.

## Use

`enter` goes deeper (DAGs, runs, tasks, log) and `esc` goes back. `?` shows
every key. The ones you will want first:

| Key | Where | What |
|---|---|---|
| `/` | DAGs, log | filter DAGs by name or tag, search the log |
| `p` | DAGs | hide paused DAGs |
| `i` | DAGs | DAG files that failed to import |
| `t` | tasks | switch between the table and a timeline |
| `[` `]` | log | previous / next try |
| `r` | anywhere | reload |
| `q` | anywhere | quit |

The mouse works too: scroll, click to select, click again to open. Times
are in UTC.

## Read-only

airpeek only sends GET requests. Both clients, the HTTP one and the MWAA
one, have the method fixed in one place, and `readonly_test.go` fails the
build if any other method, or an MWAA call that changes something, shows
up anywhere in the code.

That covers the tool. To be sure on the server side too, give it a user or
token with Airflow's Viewer role, as described for each platform above.
Airflow then rejects any change on its own.

It also tries to go easy on the webserver. The recent runs column is
loaded only for the rows on screen, at most 8 requests at a time, and
nothing refreshes by itself.

## Development

```bash
make test    # go test -race ./...
make lint    # go vet + golangci-lint
make run ARGS="--env local"
```

How the code is split:

- `internal/airflow` has the REST API calls and models. It talks through
  a one-method `Getter` interface.
- `internal/rest` and `internal/mwaa` are the two implementations of
  `Getter`.
- `internal/config` reads and writes the config file.
- `internal/tui` is the Bubble Tea UI, a stack of screens.
- `cmd/airpeek` wires them together and has `init`.

The UI tests run against an in-memory fake Airflow.