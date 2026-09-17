# Run the local model

Prosecheck can manage Ministral 3 8B without Ollama. The model and its server stay on your computer.

## Install the model

Run this command once:

```sh
prosecheck model install ministral-3-8b
```

The command downloads these pinned files:

- The `Ministral-3-8B-Instruct-2512-Q4_K_M.gguf` model from [Mistral AI](https://huggingface.co/mistralai/Ministral-3-8B-Instruct-2512-GGUF)
- A compatible server from the [Prism llama.cpp release](https://github.com/PrismML-Eng/llama.cpp/releases/tag/prism-b9599-9ca265a)

The model file is 5.20 GB. Prosecheck shows the downloaded size, percentage, and transfer speed.

If a download stops, run the install command again. Prosecheck keeps the partial file and resumes the download.

Prosecheck checks each file before it installs the file.

If you already have the exact model file, import it instead:

```sh
prosecheck model install \
  --model-file /path/to/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf \
  ministral-3-8b
```

Prosecheck still downloads and checks the matching server. It also checks the imported model.

## Enable model reviews

Add this section to `.prosecheck.json`:

```json
{
  "semantic": {
    "enabled": true,
    "runtime": "managed",
    "model": "ministral-3-8b",
    "timeout": "60s",
    "maxDiffBytes": 12000
  }
}
```

The next check starts the server when necessary. The first check can take longer while the model loads.

The server listens on `127.0.0.1`. Other computers cannot connect to it. It prefers port `11435` and selects another port when necessary.

The selected port and process details stay in the user data directory. They do not enter the repository configuration.

## Check the installation

Run a complete test:

```sh
prosecheck model doctor
```

This command starts the server and asks Ministral to review a test message.

Use these commands for routine checks:

```sh
prosecheck model status
prosecheck model logs --lines 80
prosecheck model stop
```

`model stop` stops only the server that Prosecheck started.

## Review timeouts in hooks

The default review timeout is 60 seconds. Model startup has a separate timeout.
Large staged diffs and other model requests can increase the review time.

In the default `--semantic auto` mode, `PC901` means that the semantic review did not finish.
Local rules still run. This note does not block the commit, even with `--strict`.
With `--semantic on`, a failed review returns exit code 2.

Older configuration files can still contain `"timeout": "20s"`.
An explicit value takes precedence over the default.
If reviews time out, increase `semantic.timeout` in `.prosecheck.json` to `"60s"` or `"120s"`.

To override the timeout for hooks in the current shell, run:

```sh
export PROSECHECK_TIMEOUT=120s
```

This environment variable takes precedence over the repository configuration.
It applies to `prosecheck check`, including calls from Git hooks.
The timeout must be a positive duration, such as `60s` or `2m`.

For faster reviews with less diff context, reduce `semantic.maxDiffBytes` from its default of `12000`.
A value of `0` omits the diff and disables message-versus-diff checks.

## Storage

By default, Prosecheck stores the model in the standard Hugging Face cache:

```text
~/.cache/huggingface/hub/
```

Other Hugging Face-compatible tools can reuse this model file. Prosecheck stores its server, state, and logs in the operating system's user data directory.

Set `HF_HOME` or `HF_HUB_CACHE` to select a different shared model cache.

Set `PROSECHECK_HOME` to keep all Prosecheck data in one location:

```sh
PROSECHECK_HOME=/path/to/data prosecheck model status
```

When you set `PROSECHECK_HOME`, Prosecheck stores the model below that location instead of the shared cache.

Do not point these variables at a repository. The model file is too large for Git.

## Use another local server

You can use any OpenAI-compatible server instead of the managed runtime:

```json
{
  "semantic": {
    "enabled": true,
    "runtime": "external",
    "endpoint": "http://127.0.0.1:9000/v1",
    "model": "your-model-name",
    "timeout": "60s",
    "maxDiffBytes": 12000
  }
}
```

Prosecheck does not start, stop, or change an external server.

## Privacy

Local rules never send data to a model. A model review sends the commit message and a limited staged diff to the selected server.

The managed server runs on your computer. An external endpoint can send this data elsewhere, depending on that server.
