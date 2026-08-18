# Compare local models

This comparison measures how well small local models review commit messages. It uses the current Prosecheck prompt and synthetic eval suite.

## Result

Use Ministral 3 8B Q4_K_M as the managed model.

It had the best precision, recall, F1 score, exact-match rate, and clear-message false-positive rate. A review took 2.2 seconds on average on the test computer.

Keep model findings as notes. Ministral missed every missing-reason and local-jargon case in this suite.

## Finalists

Each finalist reviewed all 30 cases three times at temperature zero.

| Model | File size | Precision | Recall | F1 | Exact matches | Clear-message false positives | Mean time |
|---|---:|---:|---:|---:|---:|---:|---:|
| Ministral 3 8B Q4_K_M | 5.20 GB | 68.0% | 63.0% | 65.4% | 46.7% | 12.5% | 2,217 ms |
| Qwen 3.5 4B Q4_K_M | 2.74 GB | 47.6% | 37.0% | 41.7% | 40.0% | 37.5% | 1,289 ms |
| Ministral 3 3B Q4_K_M | 2.15 GB | 44.4% | 14.8% | 22.2% | 30.0% | 25.0% | 518 ms |
| Bonsai 8B Q1_0 | 1.16 GB | 23.5% | 14.8% | 18.2% | 23.3% | 62.5% | 657 ms |

False positives matter in a commit hook. Ministral 3 8B Q4 reported a problem for one of eight clear messages. Qwen 3.5 4B reported a problem for three.

## Smaller Ministral quantization

We also tested a community Ministral 3 8B Q2_K file. It is 3.35 GB and averaged 580 ms per review.

Q2_K had 0% recall and 0 F1 across three runs. It returned no finding for every intended problem. This loss is too large for a 1.85 GB saving.

Mistral AI provides Q4_K_M as its smallest official GGUF file. The Q2_K file came from a third-party conversion.

## Other screened models

These models completed one screening run. We stopped testing them after a larger model performed better.

| Model | F1 | Clear-message false positives | Mean time | Reason to stop |
|---|---:|---:|---:|---|
| Qwen 3 4B Instruct 2507 Q4 | 43.4% | 87.5% | 2,835 ms | Too many false positives |
| Qwen 3.5 9B Q4 | 49.2% | 62.5% | 3,379 ms | Slower and less accurate than Ministral 8B |
| SmolLM3 3B Q4 | 32.8% | 100.0% | 933 ms | Flagged every clear message |
| Phi-4 Mini 3.8B Q4 | 28.6% | 62.5% | 870 ms | Low accuracy |
| LFM2.5 1.2B Q8 | 27.7% | 100.0% | 1,035 ms | Flagged every clear message |
| Qwen 3.5 2B Q4 | 25.5% | 50.0% | 876 ms | Returned 16 invalid responses |
| Qwen 3.5 0.8B Q8 | 11.8% | 25.0% | 421 ms | Missed most problems |
| Qwen 3 1.7B Q8 | 0.0% | 25.0% | 202 ms | Found none of the expected problems |
| Granite 4.0 1B Q8 | Not completed | Not completed | More than 6,000 ms | Too slow for a commit hook |

## Eval method

The suite contains 30 synthetic commit messages:

- Eight clear messages measure false positives.
- Eighteen cases isolate one semantic rule.
- Four cases combine two or three rules.

The suite contains 27 expected findings across `SEM001` through `SEM006`. The test used an Apple M1 Pro with 32 GB of memory, Metal, and the same local runtime for every model.

Run the eval against an OpenAI-compatible server:

```sh
go run ./cmd/prosecheck-eval \
  --endpoint http://127.0.0.1:11435/v1 \
  --model ministral-3-8b \
  --cases evals/semantic.json \
  --repeat 3
```

The result summary is in [`evals/results/2026-08-18-local-model-screen.json`](../evals/results/2026-08-18-local-model-screen.json).

## Limits

This suite is small and synthetic. Some labels require judgment. The same author created the prompt and labels.

Add anonymized real commit messages as a separate test set after a privacy review. Do not tune the prompt against that test set.
