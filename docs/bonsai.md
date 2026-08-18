# Run Bonsai locally

This setup runs 1-bit Bonsai 8B with an OpenAI-compatible server. The model stays on the local computer.

## Requirements

Install these tools:

- Git
- CMake
- A C and C++ compiler
- The `Bonsai-8B-Q1_0.gguf` model file

The model file is approximately 1.16 GB. Download it from the [Prism ML model page](https://huggingface.co/prism-ml/Bonsai-8B-gguf).

## Build the supported runtime

Clone the Prism ML fork of llama.cpp:

```sh
git clone --depth 1 https://github.com/PrismML-Eng/llama.cpp.git
cd llama.cpp
```

Configure the build on macOS:

```sh
cmake -B build -DGGML_METAL=ON -DGGML_NATIVE=ON -DLLAMA_BUILD_UI=OFF
```

Build the server:

```sh
cmake --build build -j --target llama-server
```

## Start the server

Replace `/path/to/Bonsai-8B-Q1_0.gguf` with the model path.

```sh
./build/bin/llama-server \
  -m /path/to/Bonsai-8B-Q1_0.gguf \
  --host 127.0.0.1 \
  --port 8080 \
  -ngl 99 \
  -c 8192
```

While prosecheck uses the model, keep this process open.

## Configure prosecheck

Add this section to `.prosecheck.json`:

```json
{
  "semantic": {
    "enabled": true,
    "endpoint": "http://127.0.0.1:8080/v1",
    "model": "bonsai",
    "timeout": "20s",
    "maxDiffBytes": 12000
  }
}
```

Check a context-dependent message:

```sh
prosecheck check --semantic on --message "Handle the revised path

This addresses the issue from our last meeting."
```

The reviewer reports that future readers cannot access the meeting context.

## Tested result

The Prism ML runtime loaded Bonsai 8B on an Apple M1 Pro with 32 GB of memory.

The model generated approximately 47 to 52 tokens per second. A semantic review took between 0.3 and 2.0 seconds when warm.

Ollama 0.24 downloaded the `Q1_0` model but failed to load it on the same computer. Its Metal runner rejected the tensor type.

Use the Prism ML fork until the installed Ollama release can load this model.
