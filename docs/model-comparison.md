# Compare Bonsai 4B and 8B

This comparison measures raw speed and Prosecheck review quality. The result applies to this hardware, runtime, prompt, and eval suite.

## Result

Keep Bonsai 8B as the default model for now.

The 4B model has faster prompt processing. However, it produces more findings and takes longer to complete a Prosecheck review.

The 8B model also has fewer false positives on clear messages. Neither model is accurate enough to block a commit.

## Models

The two files have different weight formats. As a result, the 4B file is only 7.2% smaller than the 8B file.

| Model | Parameters | Weight format | File size | Context |
|---|---:|---|---:|---:|
| Ternary Bonsai 4B | 4.02B | Q2_0, 2.125 bits per weight | 1.07 GB | 32,768 |
| Bonsai 8B | 8.19B | Q1_0, 1.125 bits per weight | 1.16 GB | 65,536 |

Prism reports similar general benchmark averages: 70.7 for 4B and 70.5 for 8B. These scores do not measure commit-message review.

The 8B model scores higher on IFEval, which measures instruction following. The 4B model scores higher on several reasoning and coding tasks.

See the official [4B model card](https://huggingface.co/prism-ml/Ternary-Bonsai-4B-gguf) and [8B model card](https://huggingface.co/prism-ml/Bonsai-8B-gguf).

## Local raw performance

The local benchmark used an Apple M1 Pro with 32 GB of unified memory. Both models used the same Prism build and Metal settings.

The benchmark processed 512 prompt tokens and generated 128 tokens. Each test ran five times after a warmup.

| Measure | Bonsai 4B | Bonsai 8B | Difference |
|---|---:|---:|---:|
| Prompt processing | 445.3 tokens/s | 256.2 tokens/s | 4B is 1.74x faster |
| Token generation | 49.9 tokens/s | 49.2 tokens/s | 4B is 1.4% faster |
| Peak resident memory | 1.26 GB | 1.35 GB | 4B uses 6.3% less |
| Model file | 1.07 GB | 1.16 GB | 4B uses 7.2% less |

The 4B model does not give a large memory or generation-speed saving on this computer. Its main raw advantage is prompt processing.

## Synthetic Prosecheck eval

The first suite contains 30 synthetic commit messages:

- Eight clear messages measure false positives.
- Eighteen cases isolate one semantic rule.
- Four cases combine two or three rules.

The suite contains 27 expected findings across `SEM001` through `SEM006`. Each model reviewed every case three times at temperature zero.

| Measure | Bonsai 4B | Bonsai 8B |
|---|---:|---:|
| Precision | 18.2% | 23.5% |
| Recall | 22.2% | 14.8% |
| F1 | 20.0% | 18.2% |
| Exact case matches | 16.7% | 23.3% |
| Clear-message false positives | 100.0% | 62.5% |
| Mean review time | 1,176 ms | 657 ms |
| Median review time | 1,093 ms | 315 ms |
| 95th-percentile review time | 2,176 ms | 1,320 ms |
| Protocol errors | 0 | 0 |
| Stable results across three runs | 100% | 100% |

The 4B model has slightly higher F1 because it reports more findings. It reports a false problem for every clear message.

The 8B model returns more `CLEAR` results. Therefore, it has better precision, better exact matches, and lower review latency.

The 8B model detects half of the diff conflicts without a false `SEM005` result. The 4B model does not detect a diff conflict.

Both models miss most missing reasons, hidden outcomes, local jargon, and broad subjects. The current prompt and models need more work.

## Recommendation

Use 8B if Prosecheck must choose between these models. Its lower false-positive rate is more important in a commit hook.

Keep semantic findings at information level. Do not let either model block a commit with the current quality scores.

The next model experiment must improve precision on clear messages. A better prompt can help, but prompt changes must use a separate development set.

Add anonymized real commit messages as a held-out test set after a privacy review. Do not tune the prompt against that test set.

## Reproduce the eval

Start an OpenAI-compatible server for the selected model. Then run this command from the repository root:

```sh
go run ./cmd/prosecheck-eval \
  --endpoint http://127.0.0.1:11435/v1 \
  --model bonsai-8b \
  --cases evals/semantic.json \
  --repeat 3
```

Use `--format json` if another program reads the result.

The raw result summary is in [`evals/results/2026-08-18-m1-pro.json`](../evals/results/2026-08-18-m1-pro.json).

## Limits

This suite is small and synthetic. Some labels require judgment, especially `SEM002` and `SEM003`.

The same author created the prompt and labels. Independent review can find label errors and missing cases.

These numbers compare two models with one prompt. They do not show the best possible result for either model.
