# Run Bonsai locally

prosecheck can manage Bonsai 8B without Ollama. The model and its server stay on your computer.

## Install the model

Run this command once:

```sh
prosecheck model install bonsai-8b
```

The command downloads these pinned files:

- The `Bonsai-8B-Q1_0.gguf` model from [Prism ML](https://huggingface.co/prism-ml/Bonsai-8B-gguf)
- The matching server from the [Prism llama.cpp release](https://github.com/PrismML-Eng/llama.cpp/releases/tag/prism-b9599-9ca265a)

The model is approximately 1.16 GB. prosecheck checks each file before it installs the file.

If you already have the exact model file, import it instead:

```sh
prosecheck model install --model-file /path/to/Bonsai-8B-Q1_0.gguf bonsai-8b
```

prosecheck still downloads and verifies the matching server. It also verifies the imported model.

## Enable model reviews

Add this section to `.prosecheck.json`:

```json
{
  "semantic": {
    "enabled": true,
    "runtime": "managed",
    "model": "bonsai-8b",
    "timeout": "20s",
    "maxDiffBytes": 12000
  }
}
```

The next check starts the server when necessary. The first check can take longer while the model loads.

The server uses `127.0.0.1`, so other computers cannot connect to it. It prefers port `11435` and selects another port when necessary.

The selected port and process details stay in the user data directory. They do not enter the repository configuration.

## Check the installation

Run a complete test:

```sh
prosecheck model doctor
```

This command starts the server and asks Bonsai to review a test message.

Use these commands for routine checks:

```sh
prosecheck model status
prosecheck model logs --lines 80
prosecheck model stop
```

`model stop` stops only the server that prosecheck started.

## Storage

prosecheck stores the model, server, state, and logs in the operating system's user data directory.

Set `PROSECHECK_HOME` if you need a different location:

```sh
PROSECHECK_HOME=/path/to/data prosecheck model status
```

Do not point this variable at a repository. The model file is too large for Git.

## Use another local server

You can use any OpenAI-compatible server instead of the managed runtime:

```json
{
  "semantic": {
    "enabled": true,
    "runtime": "external",
    "endpoint": "http://127.0.0.1:9000/v1",
    "model": "your-model-name",
    "timeout": "20s",
    "maxDiffBytes": 12000
  }
}
```

prosecheck does not start, stop, or change an external server.

## Privacy

Local rules never send data to a model. A model review sends the commit message and a limited staged diff to the selected server.

The managed server runs on your computer. An external endpoint can send this data elsewhere, depending on that server.
