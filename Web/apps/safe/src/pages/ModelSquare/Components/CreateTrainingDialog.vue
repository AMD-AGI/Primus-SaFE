<template>
  <el-dialog
    v-model="dialogVisible"
    title="Training"
    :close-on-click-modal="false"
    width="860"
    destroy-on-close
    @close="handleClose"
    class="training-dialog"
  >
    <div class="p-y-3 p-x-5">
      <!-- Base Model -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Base Model</span>
      </div>

      <div class="model-info-card m-b-4">
        <div class="flex items-center gap-3">
          <div v-if="props.model?.icon" class="model-icon">
            <img :src="props.model.icon" alt="" class="w-10 h-10 rounded" />
          </div>
          <div>
            <div class="font-medium">{{ props.model?.displayName || props.model?.id }}</div>
            <div class="text-xs text-gray-500 mt-1">
              <el-tag size="small" type="success">{{ props.model?.phase }}</el-tag>
              <el-tag size="small" type="primary" class="ml-1">{{ props.model?.accessMode }}</el-tag>
            </div>
          </div>
        </div>
      </div>

      <!-- Training Type -->
      <div class="flex items-center m-b-4 m-t-6">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Training Type</span>
      </div>

      <el-radio-group v-model="trainingType" class="m-b-4" :disabled="configLoading">
        <el-radio-button value="sft">SFT</el-radio-button>
        <el-radio-button value="rl">RL</el-radio-button>
      </el-radio-group>

      <!-- Unsupported warning -->
      <el-alert
        v-if="currentConfig && !currentConfig.supported"
        :title="(currentConfig as any).reason || `This model does not support ${trainingType.toUpperCase()} training`"
        type="warning"
        show-icon
        :closable="false"
        class="m-b-4"
      />

      <!-- Loading -->
      <div v-if="configLoading" class="text-center p-y-8">
        <el-icon class="is-loading" :size="24"><Loading /></el-icon>
        <div class="text-gray-500 mt-2">Loading configuration...</div>
      </div>

      <!-- ============ SFT Form ============ -->
      <el-form
        v-if="trainingType === 'sft' && sftConfig?.supported && !configLoading"
        ref="sftFormRef"
        :model="sftForm"
        :rules="sftFormRules"
        label-width="auto"
      >
        <!-- Dataset -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Dataset</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Job Name" prop="displayName">
              <el-input v-model="sftForm.displayName" placeholder="e.g. sft-qwen3-8b-alpaca" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Dataset" prop="datasetId">
              <el-select
                v-model="sftForm.datasetId"
                placeholder="Select SFT dataset"
                filterable
                class="w-full"
                :loading="datasetsLoading"
              >
                <el-option
                  v-for="ds in datasets"
                  :key="ds.datasetId"
                  :label="ds.displayName"
                  :value="ds.datasetId"
                >
                  <div class="flex justify-between items-center">
                    <span>{{ ds.displayName }}</span>
                    <span class="text-xs text-gray-400">{{ ds.totalSizeStr || '' }}</span>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Training Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Training Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="PEFT" prop="trainConfig.peft">
              <el-select v-model="sftForm.trainConfig.peft" class="w-full">
                <el-option v-for="opt in sftConfig.options.peftOptions" :key="opt" :label="opt" :value="opt" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Dataset Format" prop="trainConfig.datasetFormat">
              <el-select v-model="sftForm.trainConfig.datasetFormat" class="w-full">
                <el-option v-for="opt in sftConfig.options.datasetFormatOptions" :key="opt" :label="opt" :value="opt" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Train Iters" prop="trainConfig.trainIters">
              <el-input-number v-model="sftForm.trainConfig.trainIters" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Global Batch Size" prop="trainConfig.globalBatchSize">
              <el-input-number v-model="sftForm.trainConfig.globalBatchSize" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Micro Batch Size" prop="trainConfig.microBatchSize">
              <el-input-number v-model="sftForm.trainConfig.microBatchSize" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Seq Length" prop="trainConfig.seqLength">
              <el-input-number v-model="sftForm.trainConfig.seqLength" :min="1" :step="256" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Learning Rate" prop="trainConfig.finetuneLr">
              <el-input-number v-model="sftForm.trainConfig.finetuneLr" :min="0" :step="0.00001" :precision="6" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Advanced Training Settings -->
        <el-collapse class="m-b-4">
          <el-collapse-item title="Advanced Training Settings" name="advanced-train">
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Min LR">
                  <el-input-number v-model="sftForm.trainConfig.minLr" :min="0" :step="0.00001" :precision="6" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="LR Warmup Iters">
                  <el-input-number v-model="sftForm.trainConfig.lrWarmupIters" :min="0" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Eval Interval">
                  <el-input-number v-model="sftForm.trainConfig.evalInterval" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Save Interval">
                  <el-input-number v-model="sftForm.trainConfig.saveInterval" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Precision">
                  <el-input v-model="sftForm.trainConfig.precisionConfig" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Packed Sequence">
                  <el-switch v-model="sftForm.trainConfig.packedSequence" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="TP Size">
                  <el-input-number v-model="sftForm.trainConfig.tensorModelParallelSize" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="PP Size">
                  <el-input-number v-model="sftForm.trainConfig.pipelineModelParallelSize" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="CP Size">
                  <el-input-number v-model="sftForm.trainConfig.contextParallelSize" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Sequence Parallel">
                  <el-switch v-model="sftForm.trainConfig.sequenceParallel" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20" v-if="sftForm.trainConfig.peft === 'lora'">
              <el-col :span="12">
                <el-form-item label="LoRA Dim">
                  <el-input-number v-model="sftForm.trainConfig.peftDim" :min="0" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="LoRA Alpha">
                  <el-input-number v-model="sftForm.trainConfig.peftAlpha" :min="0" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>

        <!-- Resource Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Resource Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Image" prop="image">
              <el-input v-model="sftForm.image" placeholder="Training image" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Priority" prop="priority">
              <el-select v-model="sftForm.priority" class="w-full">
                <el-option v-for="opt in sftConfig.options.priorityOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Nodes" prop="nodeCount">
              <el-input-number v-model="sftForm.nodeCount" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="GPUs" prop="gpuCount">
              <el-input-number v-model="sftForm.gpuCount" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="CPU" prop="cpu">
              <el-input v-model="sftForm.cpu" placeholder="e.g. 128" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Memory" prop="memory">
              <el-input v-model="sftForm.memory" placeholder="e.g. 1024">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Ephemeral Storage">
              <el-input v-model="sftForm.ephemeralStorage" placeholder="e.g. 300">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Output / Export -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Output / Export</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Export model after training">
              <el-switch v-model="sftForm.exportModel" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="text-xs text-gray-400 m-b-4" style="padding-left: 2px">
          When enabled, training output will be exported to PFS and registered in Model Square
        </div>

        <!-- Advanced Settings -->
        <el-collapse class="m-b-4">
          <el-collapse-item title="Advanced Settings" name="advanced">
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Timeout (seconds)">
                  <el-input-number v-model="sftForm.timeout" :min="0" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Force Host Network">
                  <el-switch v-model="sftForm.forceHostNetwork" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>
      </el-form>

      <!-- ============ RL Form ============ -->
      <el-form
        v-if="trainingType === 'rl' && rlConfig?.supported && !configLoading"
        ref="rlFormRef"
        :model="rlForm"
        :rules="rlFormRules"
        label-width="auto"
      >
        <!-- Strategy -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Strategy</span>
        </div>

        <el-form-item>
          <el-radio-group v-model="rlForm.trainConfig.strategy">
            <el-radio-button v-for="s in rlConfig?.options.strategyOptions" :key="s" :value="s">
              {{ s === 'fsdp2' ? 'FSDP2 (8B-32B)' : 'Megatron (32B+)' }}
            </el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- Dataset -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Dataset</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Job Name" prop="displayName">
              <el-input v-model="rlForm.displayName" placeholder="e.g. rl-qwen3-8b-grpo" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Dataset" prop="datasetId">
              <el-select
                v-model="rlForm.datasetId"
                placeholder="Select RLHF dataset"
                filterable
                class="w-full"
                :loading="datasetsLoading"
              >
                <el-option
                  v-for="ds in datasets"
                  :key="ds.datasetId"
                  :label="ds.displayName"
                  :value="ds.datasetId"
                >
                  <div class="flex justify-between items-center">
                    <span>{{ ds.displayName }}</span>
                    <span class="text-xs text-gray-400">{{ ds.totalSizeStr || '' }}</span>
                  </div>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Training Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Training Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Algorithm">
              <el-select v-model="rlForm.trainConfig.algorithm" class="w-full">
                <el-option v-for="a in rlConfig?.options.algorithmOptions" :key="a" :label="a.toUpperCase()" :value="a" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Reward Type">
              <el-select v-model="rlForm.trainConfig.rewardType" class="w-full">
                <el-option v-for="r in rlConfig?.options.rewardTypeOptions" :key="r" :label="r" :value="r" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Batch Size">
              <el-input-number v-model="rlForm.trainConfig.trainBatchSize" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Max Prompt Length">
              <el-input-number v-model="rlForm.trainConfig.maxPromptLength" :min="1" :step="256" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Max Response Length">
              <el-input-number v-model="rlForm.trainConfig.maxResponseLength" :min="1" :step="256" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Total Epochs">
              <el-input-number v-model="rlForm.trainConfig.totalEpochs" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Learning Rate">
              <el-input-number v-model="rlForm.trainConfig.actorLr" :min="0" :step="0.000001" :precision="8" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Grad Clip">
              <el-input-number v-model="rlForm.trainConfig.gradClip" :min="0" :step="0.1" :precision="2" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Use KL Loss">
              <el-switch v-model="rlForm.trainConfig.useKlLoss" />
            </el-form-item>
          </el-col>
          <el-col :span="12" v-if="rlForm.trainConfig.useKlLoss">
            <el-form-item label="KL Loss Coef">
              <el-input-number v-model="rlForm.trainConfig.klLossCoef" :min="0" :step="0.001" :precision="4" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- FSDP2 Settings -->
        <template v-if="rlForm.trainConfig.strategy === 'fsdp2'">
          <el-divider content-position="left">FSDP2 Settings</el-divider>
          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="Param Offload">
                <el-switch v-model="rlForm.trainConfig.paramOffload" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Optimizer Offload">
                <el-switch v-model="rlForm.trainConfig.optimizerOffload" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="Gradient Checkpointing">
                <el-switch v-model="rlForm.trainConfig.gradientCheckpointing" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Torch Compile">
                <el-switch v-model="rlForm.trainConfig.useTorchCompile" />
              </el-form-item>
            </el-col>
          </el-row>
        </template>

        <!-- Megatron Settings -->
        <template v-if="rlForm.trainConfig.strategy === 'megatron'">
          <el-divider content-position="left">Megatron Settings</el-divider>
          <el-row :gutter="20">
            <el-col :span="8">
              <el-form-item label="TP Size">
                <el-input-number v-model="rlForm.trainConfig.megatronTpSize" :min="1" controls-position="right" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="PP Size">
                <el-input-number v-model="rlForm.trainConfig.megatronPpSize" :min="1" controls-position="right" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="CP Size">
                <el-input-number v-model="rlForm.trainConfig.megatronCpSize" :min="1" controls-position="right" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :span="8">
              <el-form-item label="EP Size">
                <el-input-number v-model="rlForm.trainConfig.megatronEpSize" :min="1" controls-position="right" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="Grad Offload">
                <el-switch v-model="rlForm.trainConfig.gradOffload" />
              </el-form-item>
            </el-col>
          </el-row>
        </template>

        <!-- Advanced Training Settings -->
        <el-collapse class="m-b-4">
          <el-collapse-item title="Advanced Training Settings" name="advanced-train">
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Mini Patch Size">
                  <el-input-number v-model="rlForm.trainConfig.miniPatchSize" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Micro Batch Size / GPU">
                  <el-input-number v-model="rlForm.trainConfig.microBatchSizePerGpu" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Rollout N">
                  <el-input-number v-model="rlForm.trainConfig.rolloutN" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Rollout TP Size">
                  <el-input-number v-model="rlForm.trainConfig.rolloutTpSize" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Rollout GPU Memory">
                  <el-input-number v-model="rlForm.trainConfig.rolloutGpuMemory" :min="0" :max="1" :step="0.1" :precision="2" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Ref Param Offload">
                  <el-switch v-model="rlForm.trainConfig.refParamOffload" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Ref Reshard After Forward">
                  <el-switch v-model="rlForm.trainConfig.refReshardAfterForward" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="Save Freq">
                  <el-input-number v-model="rlForm.trainConfig.saveFreq" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Test Freq">
                  <el-input-number v-model="rlForm.trainConfig.testFreq" :min="1" controls-position="right" />
                </el-form-item>
              </el-col>
            </el-row>
          </el-collapse-item>
        </el-collapse>

        <!-- Resource Configuration -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Resource Configuration</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Image" prop="image">
              <el-input v-model="rlForm.image" placeholder="Training image" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Priority" prop="priority">
              <el-select v-model="rlForm.priority" class="w-full">
                <el-option v-for="opt in rlConfig?.options.priorityOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Nodes" prop="nodeCount">
              <el-input-number v-model="rlForm.nodeCount" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="GPUs / Node" prop="gpuCount">
              <el-input-number v-model="rlForm.gpuCount" :min="1" controls-position="right" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="CPU" prop="cpu">
              <el-input v-model="rlForm.cpu" placeholder="e.g. 128" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Memory" prop="memory">
              <el-input v-model="rlForm.memory" placeholder="e.g. 2048">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Shared Memory" prop="sharedMemory">
              <el-input v-model="rlForm.sharedMemory" placeholder="e.g. 1024">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Ephemeral Storage">
              <el-input v-model="rlForm.ephemeralStorage" placeholder="e.g. 500">
                <template #append>Gi</template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- Output / Export -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Output / Export</span>
        </div>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="Export model after training">
              <el-switch v-model="rlForm.exportModel" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="text-xs text-gray-400 m-b-4" style="padding-left: 2px">
          When enabled, training output will be exported in HuggingFace format and registered in Model Square
        </div>
      </el-form>
    </div>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button
        type="primary"
        @click="handleSubmit"
        :loading="submitting"
        :disabled="configLoading || (currentConfig != null && !currentConfig.supported)"
      >
        {{ trainingType === 'sft' ? 'Create SFT Job' : 'Create RL Job' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import { getSftConfig, createSftJob } from '@/services/sft'
import type { SftConfigResponse, SftTrainConfig } from '@/services/sft'
import { getRlConfig, createRlJob } from '@/services/rl'
import type { RlConfigResponse, RlTrainConfig } from '@/services/rl'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import type { PlaygroundModel } from '@/services/playground'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  visible: boolean
  model: PlaygroundModel | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: [workloadId: string]
}>()

const wsStore = useWorkspaceStore()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val: boolean) => emit('update:visible', val),
})

const trainingType = ref<'sft' | 'rl'>('sft')
const sftFormRef = ref<FormInstance>()
const rlFormRef = ref<FormInstance>()
const configLoading = ref(false)
const datasetsLoading = ref(false)
const submitting = ref(false)
const sftConfig = ref<SftConfigResponse | null>(null)
const rlConfig = ref<RlConfigResponse | null>(null)
const datasets = ref<DatasetItem[]>([])

const currentConfig = computed(() => {
  return trainingType.value === 'sft' ? sftConfig.value : rlConfig.value
})

// ── SFT defaults & form ──

const defaultSftTrainConfig: SftTrainConfig = {
  peft: 'none',
  datasetFormat: 'alpaca',
  trainIters: 1000,
  globalBatchSize: 128,
  microBatchSize: 1,
  seqLength: 2048,
  finetuneLr: 0.0001,
  minLr: 0,
  lrWarmupIters: 50,
  evalInterval: 30,
  saveInterval: 50,
  precisionConfig: 'bf16_mixed',
  tensorModelParallelSize: 1,
  pipelineModelParallelSize: 1,
  contextParallelSize: 1,
  sequenceParallel: false,
  peftDim: 0,
  peftAlpha: 0,
  packedSequence: false,
}

const sftForm = reactive({
  displayName: '',
  datasetId: '',
  exportModel: true,
  image: '',
  nodeCount: 1,
  gpuCount: 8,
  cpu: '128',
  memory: '1024',
  ephemeralStorage: '300',
  priority: 1,
  trainConfig: { ...defaultSftTrainConfig },
  timeout: 0,
  forceHostNetwork: false,
})

const sftFormRules: FormRules = {
  displayName: [{ required: true, message: 'Please enter a job name', trigger: 'blur' }],
  datasetId: [{ required: true, message: 'Please select a dataset', trigger: 'change' }],
  image: [{ required: true, message: 'Image is required', trigger: 'blur' }],
  nodeCount: [{ required: true, message: 'Required', trigger: 'change' }],
  gpuCount: [{ required: true, message: 'Required', trigger: 'change' }],
  cpu: [{ required: true, message: 'Required', trigger: 'blur' }],
  memory: [{ required: true, message: 'Required', trigger: 'blur' }],
}

// ── RL defaults & form ──

const defaultRlTrainConfig: RlTrainConfig = {
  algorithm: 'grpo',
  strategy: 'fsdp2',
  rewardType: 'math',
  trainBatchSize: 128,
  maxPromptLength: 1024,
  maxResponseLength: 2048,
  actorLr: 0.000001,
  miniPatchSize: 4,
  microBatchSizePerGpu: 2,
  gradClip: 1.0,
  paramOffload: false,
  optimizerOffload: false,
  gradientCheckpointing: true,
  useTorchCompile: false,
  megatronTpSize: 4,
  megatronPpSize: 1,
  megatronCpSize: 1,
  megatronEpSize: 1,
  gradOffload: false,
  useKlLoss: true,
  klLossCoef: 0.001,
  rolloutN: 8,
  rolloutTpSize: 4,
  rolloutGpuMemory: 0.4,
  refParamOffload: true,
  refReshardAfterForward: true,
  totalEpochs: 1,
  saveFreq: 50,
  testFreq: 50,
}

const rlForm = reactive({
  displayName: '',
  datasetId: '',
  exportModel: true,
  image: '',
  nodeCount: 2,
  gpuCount: 8,
  cpu: '128',
  memory: '2048',
  sharedMemory: '1024',
  ephemeralStorage: '500',
  priority: 1,
  trainConfig: { ...defaultRlTrainConfig },
})

const rlFormRules: FormRules = {
  displayName: [{ required: true, message: 'Please enter a job name', trigger: 'blur' }],
  datasetId: [{ required: true, message: 'Please select a dataset', trigger: 'change' }],
  image: [{ required: true, message: 'Image is required', trigger: 'blur' }],
  nodeCount: [{ required: true, message: 'Required', trigger: 'change' }],
  gpuCount: [{ required: true, message: 'Required', trigger: 'change' }],
  cpu: [{ required: true, message: 'Required', trigger: 'blur' }],
  memory: [{ required: true, message: 'Required', trigger: 'blur' }],
}

// ── Config loading ──

const loadConfig = async () => {
  if (!props.visible || !props.model?.id) return

  if (trainingType.value === 'sft') {
    if (!sftConfig.value) {
      await loadSftConfig()
    } else if (sftConfig.value.supported) {
      await loadDatasets(sftConfig.value.datasetFilter)
    }
  } else {
    if (!rlConfig.value) {
      await loadRlConfig()
    } else if (rlConfig.value.supported) {
      await loadDatasets(rlConfig.value.datasetFilter)
    }
  }
}

const loadSftConfig = async () => {
  if (!props.model?.id) return
  configLoading.value = true
  try {
    const res = await getSftConfig(props.model.id, wsStore.currentWorkspaceId || '')
    sftConfig.value = res as unknown as SftConfigResponse
    if (sftConfig.value.supported) {
      const d = sftConfig.value.defaults
      sftForm.exportModel = d.exportModel
      sftForm.image = d.image
      sftForm.nodeCount = d.nodeCount
      sftForm.gpuCount = d.gpuCount
      sftForm.cpu = d.cpu
      sftForm.memory = d.memory.replace(/Gi$/i, '')
      sftForm.ephemeralStorage = d.ephemeralStorage.replace(/Gi$/i, '')
      sftForm.priority = d.priority
      sftForm.trainConfig = { ...d.trainConfig }
      await loadDatasets(sftConfig.value.datasetFilter)
    }
  } catch (error) {
    ElMessage.error('Failed to load SFT configuration')
    console.error('Failed to load SFT config:', error)
  } finally {
    configLoading.value = false
  }
}

const loadRlConfig = async (strategy?: string) => {
  if (!props.model?.id) return
  configLoading.value = true
  try {
    const res = await getRlConfig(props.model.id, {
      workspace: wsStore.currentWorkspaceId || '',
      strategy: strategy || rlForm.trainConfig.strategy,
    })
    rlConfig.value = res as unknown as RlConfigResponse
    if (rlConfig.value.supported && rlConfig.value.defaults) {
      const d = rlConfig.value.defaults
      rlForm.image = d.image
      rlForm.nodeCount = d.nodeCount
      rlForm.gpuCount = d.gpuCount
      rlForm.cpu = d.cpu
      rlForm.memory = d.memory.replace(/[Gi]+$/i, '')
      rlForm.sharedMemory = d.sharedMemory.replace(/[TGi]+$/i, '')
      rlForm.ephemeralStorage = d.ephemeralStorage.replace(/[Gi]+$/i, '')
      rlForm.exportModel = d.exportModel
      rlForm.priority = d.priority
      rlForm.trainConfig = { ...d.trainConfig }
      await loadDatasets(rlConfig.value.datasetFilter)
    }
  } catch (error) {
    ElMessage.error('Failed to load RL configuration')
    console.error('Failed to load RL config:', error)
  } finally {
    configLoading.value = false
  }
}

const loadDatasets = async (filter: { datasetType: string; workspace: string }) => {
  datasetsLoading.value = true
  try {
    const res = await getDatasets({
      datasetType: filter.datasetType,
      workspace: filter.workspace || wsStore.currentWorkspaceId,
    })
    datasets.value =
      (res as unknown as { items: DatasetItem[] }).items?.filter((d) => d.status === 'Ready') || []
  } catch (error) {
    console.error('Failed to load datasets:', error)
    ElMessage.error('Failed to load datasets')
  } finally {
    datasetsLoading.value = false
  }
}

// ── Watchers ──

watch(
  () => props.visible,
  async (val) => {
    if (val && props.model) {
      await loadConfig()
    }
  },
)

watch(trainingType, async () => {
  if (!props.visible || !props.model) return
  await loadConfig()
})

watch(
  () => rlForm.trainConfig.strategy,
  async (newStrategy, oldStrategy) => {
    if (configLoading.value || !props.visible) return
    if (trainingType.value === 'rl' && newStrategy !== oldStrategy && rlConfig.value) {
      const savedName = rlForm.displayName
      const savedDataset = rlForm.datasetId
      await loadRlConfig(newStrategy)
      rlForm.displayName = savedName
      rlForm.datasetId = savedDataset
    }
  },
)

// ── Submit ──

const handleSubmit = async () => {
  if (trainingType.value === 'sft') {
    await submitSftJob()
  } else {
    await submitRlJob()
  }
}

const submitSftJob = async () => {
  if (!sftFormRef.value || !props.model) return
  await sftFormRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      const res = await createSftJob({
        displayName: sftForm.displayName,
        modelId: props.model!.id,
        datasetId: sftForm.datasetId,
        workspace: wsStore.currentWorkspaceId || '',
        exportModel: sftForm.exportModel,
        image: sftForm.image,
        nodeCount: sftForm.nodeCount,
        gpuCount: sftForm.gpuCount,
        cpu: sftForm.cpu,
        memory: `${sftForm.memory}Gi`,
        ephemeralStorage: `${sftForm.ephemeralStorage}Gi`,
        priority: sftForm.priority,
        trainConfig: { ...sftForm.trainConfig },
        timeout: sftForm.timeout || undefined,
        forceHostNetwork: sftForm.forceHostNetwork || undefined,
      })
      const result = res as unknown as { workloadId: string }
      ElMessage.success('SFT job created successfully')
      emit('success', result.workloadId)
      handleClose()
    } catch (error) {
      console.error('Failed to create SFT job:', error)
      ElMessage.error((error as Error).message || 'Failed to create SFT job')
    } finally {
      submitting.value = false
    }
  })
}

const submitRlJob = async () => {
  if (!rlFormRef.value || !props.model) return
  await rlFormRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      const res = await createRlJob({
        displayName: rlForm.displayName,
        modelId: props.model!.id,
        datasetId: rlForm.datasetId,
        workspace: wsStore.currentWorkspaceId || '',
        exportModel: rlForm.exportModel,
        image: rlForm.image,
        nodeCount: rlForm.nodeCount,
        gpuCount: rlForm.gpuCount,
        cpu: rlForm.cpu,
        memory: `${rlForm.memory}Gi`,
        sharedMemory: `${rlForm.sharedMemory}Gi`,
        ephemeralStorage: `${rlForm.ephemeralStorage}Gi`,
        priority: rlForm.priority,
        trainConfig: { ...rlForm.trainConfig },
      })
      const result = res as unknown as { workloadId: string }
      ElMessage.success('RL job created successfully')
      emit('success', result.workloadId)
      handleClose()
    } catch (error) {
      console.error('Failed to create RL job:', error)
      ElMessage.error((error as Error).message || 'Failed to create RL job')
    } finally {
      submitting.value = false
    }
  })
}

const handleClose = () => {
  sftFormRef.value?.resetFields()
  rlFormRef.value?.resetFields()
  sftConfig.value = null
  rlConfig.value = null
  datasets.value = []
  trainingType.value = 'sft'
  sftForm.displayName = ''
  sftForm.datasetId = ''
  rlForm.displayName = ''
  rlForm.datasetId = ''
  Object.assign(sftForm.trainConfig, defaultSftTrainConfig)
  Object.assign(rlForm.trainConfig, defaultRlTrainConfig)
  emit('update:visible', false)
}
</script>

<style scoped lang="scss">
.model-info-card {
  padding: 12px 16px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  border: 1px solid var(--el-border-color-lighter);
}

.training-dialog :deep(.el-dialog__body) {
  max-height: 65vh;
  overflow-y: auto;
}

:deep(.el-input-number) {
  width: 100%;
}

:deep(.el-form-item) {
  margin-bottom: 18px;
}
</style>
