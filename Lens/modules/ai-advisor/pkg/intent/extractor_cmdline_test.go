// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"testing"
)

// TestCases covers 20+ real workload patterns across all four cmdline modes:
//   Mode A: cmdline-rich (vLLM, TGI, Triton, torchrun, etc.)
//   Mode B: config-based (torchrun + config.yaml)
//   Mode C: code-only (python train.py)
//   Mode D: shell-wrapped (bash run.sh -> expand to torchrun)

func TestExtract_ServingFrameworks(t *testing.T) {
	e := NewCmdlineExtractor()

	tests := []struct {
		name      string
		cmdline   string
		args      []string
		wantFW    string
		wantCat   Category
		wantModel string
	}{
		// --- Workload #1: vLLM with Llama-3-70B ---
		{
			name:    "vLLM_Llama3_70B",
			cmdline: "python -m vllm.entrypoints.openai.api_server",
			args: []string{
				"--model", "/models/meta-llama/Llama-3-70B-Instruct",
				"--tensor-parallel-size", "8",
				"--quantization", "awq",
			},
			wantFW:    "vllm",
			wantCat:   CategoryInference,
			wantModel: "/models/meta-llama/Llama-3-70B-Instruct",
		},
		// --- Workload #2: vLLM with Qwen ---
		{
			name:    "vLLM_Qwen2_72B",
			cmdline: "python -m vllm.entrypoints.openai.api_server",
			args: []string{
				"--model", "Qwen/Qwen2-72B-Instruct",
				"--tp", "4",
			},
			wantFW:    "vllm",
			wantCat:   CategoryInference,
			wantModel: "Qwen/Qwen2-72B-Instruct",
		},
		// --- Workload #3: TGI with Mixtral ---
		{
			name:    "TGI_Mixtral_8x7B",
			cmdline: "text-generation-launcher",
			args: []string{
				"--model-id", "mistralai/Mixtral-8x7B-v0.1",
				"--num-shard", "4",
			},
			wantFW:    "tgi",
			wantCat:   CategoryInference,
			wantModel: "mistralai/Mixtral-8x7B-v0.1",
		},
		// --- Workload #4: Triton Inference Server ---
		{
			name:    "Triton_Generic",
			cmdline: "tritonserver",
			args: []string{
				"--model-repository=/models",
				"--http-port=8000",
			},
			wantFW:  "triton",
			wantCat: CategoryInference,
		},
		// --- Workload #5: SGLang ---
		{
			name:    "SGLang_DeepSeek",
			cmdline: "python -m sglang.launch_server",
			args: []string{
				"--model-path", "deepseek-ai/deepseek-coder-33b-instruct",
				"--tp", "2",
			},
			wantFW:    "sglang",
			wantCat:   CategoryInference,
			wantModel: "deepseek-ai/deepseek-coder-33b-instruct",
		},
		// --- Workload #6: llama.cpp ---
		{
			name:    "LlamaCpp_GGUF",
			cmdline: "llama-server",
			args: []string{
				"-m", "/models/llama-3-8b-instruct.Q4_K_M.gguf",
				"--host", "0.0.0.0",
				"--port", "8080",
			},
			wantFW:    "llama_cpp",
			wantCat:   CategoryInference,
			wantModel: "/models/llama-3-8b-instruct.Q4_K_M.gguf",
		},
		// --- Workload #7: TorchServe ---
		{
			name:    "TorchServe",
			cmdline: "torchserve",
			args: []string{
				"--start",
				"--model-store", "/home/model-store",
			},
			wantFW:  "torchserve",
			wantCat: CategoryInference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Extract(tt.cmdline, tt.args)

			if result.ServingFramework != tt.wantFW {
				t.Errorf("serving_framework: got %q, want %q", result.ServingFramework, tt.wantFW)
			}
			if tt.wantCat != "" && result.Category != tt.wantCat {
				t.Errorf("category: got %q, want %q", result.Category, tt.wantCat)
			}
			if tt.wantModel != "" && result.ModelPath != tt.wantModel {
				t.Errorf("model_path: got %q, want %q", result.ModelPath, tt.wantModel)
			}
		})
	}
}

func TestExtract_TrainingMethods(t *testing.T) {
	e := NewCmdlineExtractor()

	tests := []struct {
		name       string
		cmdline    string
		args       []string
		wantMethod TrainingMethod
		wantCat    Category
		wantModel  string
	}{
		// --- Workload #8: SFT with LoRA ---
		{
			name:    "SFT_LoRA_HFTrainer",
			cmdline: "python train_sft.py",
			args: []string{
				"--model_name_or_path", "meta-llama/Llama-3-8B",
				"--use_lora",
				"--lora_r", "16",
				"--learning_rate", "2e-5",
				"--per_device_train_batch_size", "4",
				"--num_train_epochs", "3",
				"--output_dir", "/output/sft-llama",
			},
			wantMethod: MethodLoRA,
			wantCat:    CategoryFineTuning,
			wantModel:  "meta-llama/Llama-3-8B",
		},
		// --- Workload #9: QLoRA ---
		{
			name:    "QLoRA_Llama",
			cmdline: "python finetune_qlora.py",
			args: []string{
				"--model_name_or_path", "meta-llama/Llama-3-8B",
				"--use_qlora",
				"--bits", "4",
				"--batch_size", "8",
			},
			wantMethod: MethodQLoRA,
			wantCat:    CategoryFineTuning,
			wantModel:  "meta-llama/Llama-3-8B",
		},
		// --- Workload #10: DPO training ---
		{
			name:    "DPO_Mistral",
			cmdline: "python train_dpo.py",
			args: []string{
				"--model_name_or_path", "mistralai/Mistral-7B-v0.1",
				"--dpo_beta", "0.1",
				"--learning_rate", "5e-7",
			},
			wantMethod: MethodDPO,
			wantCat:    CategoryFineTuning,
			wantModel:  "mistralai/Mistral-7B-v0.1",
		},
		// --- Workload #11: RLHF with reward model ---
		{
			name:    "RLHF_Llama",
			cmdline: "python train_rlhf.py",
			args: []string{
				"--model_name_or_path", "meta-llama/Llama-3-8B",
				"--reward_model", "/models/reward-model",
				"--learning_rate", "1e-6",
			},
			wantMethod: MethodRLHF,
			wantCat:    CategoryFineTuning,
			wantModel:  "meta-llama/Llama-3-8B",
		},
		// --- Workload #12: Megatron pre-training ---
		{
			name:    "Megatron_Pretrain",
			cmdline: "python pretrain_gpt.py",
			args: []string{
				"--tensor_model_parallel_size", "8",
				"--pipeline_model_parallel_size", "2",
				"--batch_size", "64",
				"--lr", "1.5e-4",
				"--epochs", "2",
			},
			wantMethod: MethodPreTraining,
			wantCat:    CategoryPreTraining,
		},
		// --- Workload #13: SFT with HF trainer ---
		{
			name:    "SFT_HFTrainer",
			cmdline: "python -m sft_trainer",
			args: []string{
				"--model_name_or_path", "Qwen/Qwen2-7B",
				"--data_path", "/data/alpaca",
				"--output_dir", "/output",
			},
			wantMethod: MethodSFT,
			wantCat:    CategoryFineTuning,
			wantModel:  "Qwen/Qwen2-7B",
		},
		// --- Workload #14: Torchrun pre-training ---
		{
			name:    "Torchrun_Pretrain",
			cmdline: "torchrun --nproc_per_node 8 pretrain_main.py",
			args: []string{
				"--do_pretrain",
				"--model", "/models/gpt-neox-20b",
				"--batch_size", "32",
			},
			wantMethod: MethodPreTraining,
			wantCat:    CategoryPreTraining,
			wantModel:  "/models/gpt-neox-20b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Extract(tt.cmdline, tt.args)

			if result.Method != tt.wantMethod {
				t.Errorf("method: got %q, want %q", result.Method, tt.wantMethod)
			}
			if tt.wantCat != "" && result.Category != tt.wantCat {
				t.Errorf("category: got %q, want %q", result.Category, tt.wantCat)
			}
			if tt.wantModel != "" && result.ModelPath != tt.wantModel {
				t.Errorf("model_path: got %q, want %q", result.ModelPath, tt.wantModel)
			}
		})
	}
}

func TestExtract_ParallelismAndHyperparams(t *testing.T) {
	e := NewCmdlineExtractor()

	tests := []struct {
		name    string
		cmdline string
		args    []string
		wantTP  int
		wantPP  int
		wantDP  int
		wantLR  float64
		wantBS  int
	}{
		// --- Workload #15: DeepSpeed SFT with full parallelism ---
		{
			name:    "DeepSpeed_Full_Parallel",
			cmdline: "deepspeed train_sft.py",
			args: []string{
				"--model_name_or_path", "meta-llama/Llama-3-70B",
				"--use_lora",
				"--zero_stage", "3",
				"--nproc_per_node", "8",
				"--learning_rate", "2e-5",
				"--per_device_train_batch_size", "4",
			},
			wantDP: 8,
			wantLR: 2e-5,
			wantBS: 4,
		},
		// --- Workload #16: Megatron 3D parallelism ---
		{
			name:    "Megatron_3D_Parallel",
			cmdline: "python pretrain_gpt.py",
			args: []string{
				"--tensor_model_parallel_size", "4",
				"--pipeline_model_parallel_size", "2",
				"--nproc_per_node", "8",
				"--lr", "1e-4",
				"--batch_size", "128",
				"--gradient_accumulation_steps", "4",
			},
			wantTP: 4,
			wantPP: 2,
			wantDP: 8,
			wantLR: 1e-4,
			wantBS: 128,
		},
		// --- Workload #17: FSDP training ---
		{
			name:    "FSDP_Training",
			cmdline: "accelerate launch train.py",
			args: []string{
				"--fsdp",
				"--fsdp_config", "/workspace/fsdp_config.json",
				"--model_name_or_path", "meta-llama/Llama-3-8B",
				"--learning_rate", "5e-5",
			},
			wantLR: 5e-5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Extract(tt.cmdline, tt.args)

			if result.Parallelism != nil {
				if tt.wantTP > 0 && result.Parallelism.TensorParallel != tt.wantTP {
					t.Errorf("tp: got %d, want %d", result.Parallelism.TensorParallel, tt.wantTP)
				}
				if tt.wantPP > 0 && result.Parallelism.PipeParallel != tt.wantPP {
					t.Errorf("pp: got %d, want %d", result.Parallelism.PipeParallel, tt.wantPP)
				}
				if tt.wantDP > 0 && result.Parallelism.DataParallel != tt.wantDP {
					t.Errorf("dp: got %d, want %d", result.Parallelism.DataParallel, tt.wantDP)
				}
			} else if tt.wantTP > 0 || tt.wantPP > 0 || tt.wantDP > 0 {
				t.Error("parallelism is nil but expected values")
			}

			if result.HyperParams != nil {
				if tt.wantLR > 0 && result.HyperParams.LearningRate != tt.wantLR {
					t.Errorf("lr: got %v, want %v", result.HyperParams.LearningRate, tt.wantLR)
				}
				if tt.wantBS > 0 && result.HyperParams.BatchSize != tt.wantBS {
					t.Errorf("batch_size: got %d, want %d", result.HyperParams.BatchSize, tt.wantBS)
				}
			} else if tt.wantLR > 0 || tt.wantBS > 0 {
				t.Error("hyperparams is nil but expected values")
			}
		})
	}
}

func TestExtract_ScriptNameInference(t *testing.T) {
	e := NewCmdlineExtractor()

	tests := []struct {
		name    string
		cmdline string
		args    []string
		wantCat Category
	}{
		// --- Workload #18: eval script ---
		{
			name:    "Eval_Script",
			cmdline: "python evaluate_model.py",
			args:    []string{"--model", "meta-llama/Llama-3-8B"},
			wantCat: CategoryEvaluation,
		},
		// --- Workload #19: benchmark ---
		{
			name:    "Benchmark",
			cmdline: "python benchmark_throughput.py",
			args:    []string{"--model", "/models/llama-3-70b"},
			wantCat: CategoryEvaluation,
		},
		// --- Workload #20: data preprocessing ---
		{
			name:    "Data_Preprocessing",
			cmdline: "python tokenize_dataset.py",
			args:    []string{"--input", "/data/raw", "--output", "/data/processed"},
			wantCat: CategoryDataProcessing,
		},
		// --- Workload #21: serve script ---
		{
			name:    "Serve_Script",
			cmdline: "python serve_model.py",
			args:    []string{"--model", "/models/llama-3"},
			wantCat: CategoryServing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.Extract(tt.cmdline, tt.args)
			if result.Category != tt.wantCat {
				t.Errorf("category: got %q, want %q", result.Category, tt.wantCat)
			}
		})
	}
}

func TestExtract_ConfigPaths(t *testing.T) {
	e := NewCmdlineExtractor()

	// --- Workload #22: Config-based training ---
	result := e.Extract("torchrun --nproc_per_node=8 train.py", []string{
		"--config", "/workspace/config.yaml",
		"--deepspeed", "/workspace/ds_config.json",
	})

	if len(result.ConfigPaths) != 2 {
		t.Errorf("config_paths: got %d, want 2", len(result.ConfigPaths))
	}

	// --- Workload #23: Multiple config files ---
	result = e.Extract("python train_sft.py", []string{
		"--config_file", "/workspace/sft_config.yaml",
		"--ds_config", "/workspace/ds_z3.json",
		"--fsdp_config", "/workspace/fsdp.json",
		"--data_path", "/data/alpaca",
		"--output_dir", "/output/sft-run-1",
	})

	if len(result.ConfigPaths) != 3 {
		t.Errorf("config_paths: got %d, want 3", len(result.ConfigPaths))
	}
	if result.DataPath != "/data/alpaca" {
		t.Errorf("data_path: got %q, want %q", result.DataPath, "/data/alpaca")
	}
	if result.OutputDir != "/output/sft-run-1" {
		t.Errorf("output_dir: got %q, want %q", result.OutputDir, "/output/sft-run-1")
	}
}

func TestExtract_Coverage(t *testing.T) {
	e := NewCmdlineExtractor()

	// High coverage: vLLM with all info
	result := e.Extract("python -m vllm.entrypoints.openai.api_server", []string{
		"--model", "/models/meta-llama/Llama-3-70B-Instruct",
		"--tp", "8",
	})
	if result.Coverage < 0.4 {
		t.Errorf("vLLM coverage too low: %v", result.Coverage)
	}

	// Low coverage: bare python script
	result = e.Extract("python", []string{"script.py"})
	if result.Coverage > 0.1 {
		t.Errorf("bare script coverage too high: %v", result.Coverage)
	}
}
