<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <div class="agent-page-container">
    <!-- Page title -->
    <div class="px-4 pt-4 mb-4" style="display: flex; justify-content: flex-start">
      <el-text class="textx-18 font-500" tag="b">{{ modelName || 'Playground' }}</el-text>
    </div>

    <!-- Main container with left panel and chat -->
    <div class="flex gap-4 px-4 pb-4" style="flex: 1; overflow: hidden">
      <!-- Left panel for parameters -->
      <el-card
        class="parameters-panel safe-card"
        shadow="hover"
        style="width: 320px; height: 100%; display: flex; flex-direction: column"
      >
        <div class="params-content" style="overflow-y: auto; flex: 1">
          <!-- Service Selection -->
          <div class="param-item">
            <label class="param-label">
              Service
              <el-tooltip content="Select the inference service or model">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-select
              v-model="serviceId"
              placeholder="Select service"
              class="w-full"
              :loading="loadingServices"
              @change="handleServiceChange"
            >
              <el-option
                v-for="service in servicesList"
                :key="service.id"
                :label="service.displayName"
                :value="service.id"
              >
                <div class="flex items-center justify-between">
                  <span>{{ service.displayName }}</span>
                  <el-tag v-if="service.phase" size="small" :type="getPhaseType(service.phase)">
                    {{ service.phase }}
                  </el-tag>
                </div>
              </el-option>
            </el-select>
          </div>

          <!-- Model Name -->
          <div class="param-item">
            <label class="param-label">
              Model Name
              <el-tooltip content="Enter model name">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-input v-model="modelName" placeholder="Enter model name" />
          </div>

          <!-- System Prompt -->
          <div class="param-item">
            <label class="param-label">
              System Prompt
              <el-tooltip content="Define the assistant's behavior and personality">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-input
              v-model="systemPrompt"
              type="textarea"
              :rows="3"
              placeholder="Enter system prompt..."
              resize="none"
              class="system-prompt-input"
            />
          </div>

          <!-- Compare Mode Toggle -->
          <div class="param-item">
            <label class="param-label">
              Compare Mode
              <el-tooltip content="Enable to compare responses from two different models">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-switch v-model="compareMode" />
          </div>

          <!-- Debug Mode Toggle -->
          <div class="param-item">
            <label class="param-label">
              Debug Mode
              <el-tooltip content="Show raw content and parsing results">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-switch v-model="debugMode" />
          </div>

          <!-- Secondary Service (for comparison) -->
          <div class="param-item" v-if="compareMode">
            <label class="param-label">
              Compare Service
              <el-tooltip content="Select a second inference service for comparison">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-select
              v-model="secondaryServiceId"
              placeholder="Select compare service"
              class="w-full"
              :loading="loadingServices"
              @change="handleSecondaryServiceChange"
            >
              <el-option
                v-for="svc in servicesList"
                :key="svc.id"
                :label="svc.displayName"
                :value="svc.id"
                :disabled="svc.id === serviceId"
              >
                <div class="flex items-center justify-between">
                  <span>{{ svc.displayName }}</span>
                  <div class="flex items-center gap-2">
                    <el-tag v-if="svc.phase" size="small" :type="getPhaseType(svc.phase)">
                      {{ svc.phase }}
                    </el-tag>
                    <el-tag v-if="svc.id === serviceId" type="warning" size="small">
                      Primary
                    </el-tag>
                  </div>
                </div>
              </el-option>
            </el-select>
          </div>

          <el-divider />

          <!-- Max Tokens -->
          <div class="param-item">
            <label class="param-label">Max Tokens</label>
            <div class="flex items-center gap-2">
              <el-slider
                v-model="chatParams.maxTokens"
                :min="1"
                :max="maxTokensLimit"
                :show-tooltip="false"
                class="flex-1"
              />
              <el-input-number
                v-model="chatParams.maxTokens"
                :min="1"
                :max="maxTokensLimit"
                :controls="false"
                size="small"
                style="width: 80px"
              />
            </div>
          </div>

          <!-- Temperature -->
          <div class="param-item">
            <label class="param-label">
              Temperature
              <el-tooltip content="Controls randomness: 0 means focused, 1 means creative">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <div class="flex items-center gap-2">
              <el-slider
                v-model="chatParams.temperature"
                :min="0"
                :max="1"
                :step="0.01"
                :show-tooltip="false"
                class="flex-1"
              />
              <el-input-number
                v-model="chatParams.temperature"
                :min="0"
                :max="1"
                :step="0.01"
                :precision="2"
                :controls="false"
                size="small"
                style="width: 80px"
              />
            </div>
          </div>

          <!-- Top-p -->
          <div class="param-item">
            <label class="param-label">
              Top-p
              <el-tooltip content="Controls diversity via nucleus sampling">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <div class="flex items-center gap-2">
              <el-slider
                v-model="chatParams.topP"
                :min="0"
                :max="1"
                :step="0.01"
                :show-tooltip="false"
                class="flex-1"
              />
              <el-input-number
                v-model="chatParams.topP"
                :min="0"
                :max="1"
                :step="0.01"
                :precision="2"
                :controls="false"
                size="small"
                style="width: 80px"
              />
            </div>
          </div>

          <!-- Frequency Penalty -->
          <div class="param-item">
            <label class="param-label">
              Frequency Penalty
              <el-tooltip content="Reduces repetition of tokens">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <div class="flex items-center gap-2">
              <el-slider
                v-model="chatParams.frequencyPenalty"
                :min="-2"
                :max="2"
                :step="0.01"
                :show-tooltip="false"
                class="flex-1"
              />
              <el-input-number
                v-model="chatParams.frequencyPenalty"
                :min="-2"
                :max="2"
                :step="0.01"
                :precision="2"
                :controls="false"
                size="small"
                style="width: 80px"
              />
            </div>
          </div>

          <!-- Enable Thinking -->
          <!-- <div class="param-item">
            <label class="param-label">
              Enable Thinking
              <el-tooltip content="Enable model reasoning process">
                <el-icon class="ml-1 text-gray-400"><QuestionFilled /></el-icon>
              </el-tooltip>
            </label>
            <el-select v-model="chatParams.enableThinking" placeholder="Select" class="w-full">
              <el-option label="Disabled" :value="false" />
              <el-option label="Enabled" :value="true" />
            </el-select>
          </div> -->

          <!-- Thinking Budget -->
          <!-- <div class="param-item" v-if="chatParams.enableThinking">
            <label class="param-label">Thinking Budget</label>
            <el-input-number
              v-model="chatParams.thinkingBudget"
              :min="1"
              :max="100000"
              :controls="false"
              class="w-full"
            />
          </div> -->
        </div>
      </el-card>

      <!-- Main chat area -->
      <div
        v-if="!compareMode"
        class="flex-1"
        style="height: 100%; display: flex; flex-direction: column"
      >
        <!-- Single chat mode -->
        <el-card
          class="main-chat-card safe-card"
          style="flex: 1; display: flex; flex-direction: column; overflow: hidden"
          shadow="hover"
        >
          <template #header>
            <div class="flex justify-between items-center">
              <div class="flex items-center gap-3">
                <div class="model-info flex items-center gap-2">
                  <div class="model-indicator"></div>
                  <el-text class="font-600 text-base">{{ modelName }}</el-text>
                </div>
                <el-tag effect="light" type="info" v-if="currentSessionId" class="session-tag">
                  Session #{{ currentSessionId }}
                </el-tag>
              </div>
              <div class="flex items-center gap-3">
                <el-button plain @click="toggleHistory" class="header-btn">
                  <i i="ep-clock" class="mr-1" />
                  History
                </el-button>
                <el-button
                  plain
                  type="primary"
                  @click="saveSession"
                  :disabled="messages.length === 0"
                  :loading="savingSession"
                  class="header-btn"
                >
                  <i i="ep-document" class="mr-1" v-if="!savingSession" />
                  {{ savingSession ? 'Saving...' : 'Save' }}
                </el-button>
                <el-button
                  plain
                  type="danger"
                  @click="clearChat"
                  :disabled="messages.length === 0"
                  class="header-btn"
                >
                  <i i="ep-delete" class="mr-1" />
                  Clear Chat
                </el-button>
              </div>
            </div>
          </template>

          <!-- Content -->
          <div
            class="chat-content flex flex-col"
            :class="{ 'justify-center': messages.length === 0 }"
            style="flex: 1"
          >
            <!-- Messages -->
            <div
              v-if="messages.length > 0"
              ref="messagesContainer"
              class="messages-container flex-1 overflow-y-auto pr-2"
            >
              <!-- Messages list -->
              <div class="space-y-4 py-4">
                <div
                  v-for="(msg, index) in messages"
                  :key="index"
                  class="flex message-row"
                  :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
                >
                  <div
                    class="max-w-[80%] flex"
                    :class="msg.role === 'user' ? 'flex-row-reverse' : 'flex-row'"
                  >
                    <!-- Avatar -->
                    <div
                      class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm overflow-hidden"
                      :class="
                        msg.role === 'user'
                          ? 'bg-primary/10 ml-3'
                          : 'bg-gray-100 dark:bg-gray-700 mr-3'
                      "
                    >
                      <template v-if="msg.role === 'user'">
                        <i i="ep-user" class="text-primary text-lg" />
                      </template>
                      <template v-else>
                        <img
                          v-if="modelIcon"
                          :src="modelIcon"
                          alt="AI"
                          class="w-full h-full object-cover"
                          @error="(e) => handleIconError(e)"
                        />
                        <i v-else i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                      </template>
                    </div>

                    <!-- Message content -->
                    <div class="flex flex-col max-w-[calc(100%-52px)]">
                      <!-- Header with name and time -->
                      <div class="flex items-center gap-2 mb-1">
                        <span
                          class="text-xs font-500"
                          :class="
                            msg.role === 'user'
                              ? 'text-primary'
                              : 'text-gray-600 dark:text-gray-400'
                          "
                        >
                          {{ msg.role === 'user' ? 'You' : modelName }}
                        </span>
                        <span class="text-xs text-gray-400">
                          {{ formatMsgTime(msg.timestamp) }}
                        </span>
                      </div>

                      <!-- Message bubble with content -->
                      <div
                        class="message-bubble px-4 py-3 rounded-xl shadow-md border text-sm leading-relaxed group relative"
                        :class="
                          msg.role === 'user'
                            ? 'bg-blue-50/80 border-blue-200/50 dark:bg-blue-900/30 dark:border-blue-700/50'
                            : 'bg-white/90 dark:bg-gray-800/90 border-gray-200/50 dark:border-gray-700/50'
                        "
                      >
                        <!-- Model comparison display (side by side) -->
                        <div
                          v-if="msg.choices && msg.choices.length > 1 && msg.choices[0].modelName"
                          class="compare-container"
                        >
                          <div class="compare-header mb-3">
                            <i class="i-ep-files text-primary mr-1"></i>
                            <span class="text-xs font-500 text-gray-600">Model Comparison</span>
                          </div>
                          <div class="compare-responses">
                            <div
                              v-for="(choice, i) in msg.choices"
                              :key="i"
                              class="compare-response-item"
                            >
                              <div class="response-header">
                                <span class="model-name">{{
                                  choice.modelName || `Response ${i + 1}`
                                }}</span>
                                <el-tag
                                  v-if="choice.finish_reason === 'error'"
                                  type="danger"
                                  size="small"
                                >
                                  Error
                                </el-tag>
                                <el-tag v-else-if="choice.finish_reason" type="info" size="small">
                                  {{ choice.finish_reason }}
                                </el-tag>
                              </div>
                              <div class="response-content">
                                <!-- Thinking section in comparison -->
                                <div
                                  v-if="
                                    parseThinkingContent(choice.content).thinkingContent &&
                                    (autoExpandChoiceThinking(choice), true)
                                  "
                                  class="thinking-section thinking-section-compare"
                                >
                                  <div
                                    class="thinking-header"
                                    @click="() => toggleChoiceThinking(choice)"
                                  >
                                    <i class="i-ep-view text-sm" />
                                    <span class="thinking-label">Thinking</span>
                                    <el-button text size="small" class="thinking-toggle">
                                      {{ choice.thinkingExpanded ? 'Collapse' : 'Expand' }}
                                      <i
                                        :class="
                                          choice.thinkingExpanded
                                            ? 'i-ep-arrow-up-bold'
                                            : 'i-ep-arrow-down-bold'
                                        "
                                        class="ml-1 text-xs"
                                      />
                                    </el-button>
                                  </div>
                                  <div v-if="choice.thinkingExpanded" class="thinking-content">
                                    <div
                                      class="markdown-body"
                                      v-html="
                                        formatMessage(
                                          parseThinkingContent(choice.content).thinkingContent,
                                        )
                                      "
                                    />
                                    <span
                                      v-if="
                                        streamingMessage === msg &&
                                        !choice.finish_reason &&
                                        !parseThinkingContent(choice.content).responseContent
                                      "
                                      class="typing-cursor"
                                      >▊</span
                                    >
                                  </div>
                                </div>
                                <!-- Response section in comparison -->
                                <div class="markdown-body">
                                  <span
                                    v-html="
                                      formatMessage(
                                        parseThinkingContent(choice.content).responseContent ||
                                          choice.content,
                                      )
                                    "
                                  />
                                  <span
                                    v-if="streamingMessage === msg && !choice.finish_reason"
                                    class="typing-cursor"
                                    >▊</span
                                  >
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                        <!-- Multiple choices display (original) -->
                        <div
                          v-else-if="msg.choices && msg.choices.length > 1"
                          class="choices-container"
                        >
                          <div class="choices-header mb-2">
                            <i class="i-ep-magic-stick text-primary mr-1"></i>
                            <span class="text-xs text-gray-500"
                              >{{ msg.choices.length }} responses generated - Select the best
                              one:</span
                            >
                          </div>
                          <div class="choices-tabs mb-3">
                            <el-radio-group
                              v-model="msg.selectedChoiceIndex"
                              @change="(val: number) => selectChoice(msg, val)"
                              size="small"
                            >
                              <el-radio-button
                                v-for="(choice, i) in msg.choices"
                                :key="i"
                                :label="i"
                              >
                                <span v-if="choice.modelName">{{ choice.modelName }}</span>
                                <span v-else>Response {{ i + 1 }}</span>
                              </el-radio-button>
                            </el-radio-group>
                          </div>
                          <div class="choice-content">
                            <div
                              class="markdown-body"
                              v-html="
                                formatMessage(msg.choices[msg.selectedChoiceIndex || 0].content)
                              "
                            />
                            <div
                              v-if="msg.choices[msg.selectedChoiceIndex || 0].finish_reason"
                              class="text-xs text-gray-400 mt-2"
                            >
                              <i class="i-ep-info-filled mr-1"></i>
                              Finish reason:
                              {{ msg.choices[msg.selectedChoiceIndex || 0].finish_reason }}
                            </div>
                          </div>
                        </div>
                        <!-- Single message display -->
                        <div v-else>
                          <!-- Debug info -->
                          <div v-if="debugMode && msg.role === 'assistant'" class="debug-info">
                            <div class="debug-section">
                              <strong>Raw Content (first 200 chars):</strong>
                              <pre>{{ msg.content.substring(0, 200) }}</pre>
                            </div>
                            <div class="debug-section">
                              <strong>Thinking Content:</strong>
                              <pre>{{
                                parseThinkingContent(msg.content).thinkingContent.substring(
                                  0,
                                  100,
                                ) || 'None'
                              }}</pre>
                            </div>
                            <div class="debug-section">
                              <strong>Response Content (first 100 chars):</strong>
                              <pre>{{
                                parseThinkingContent(msg.content).responseContent.substring(
                                  0,
                                  100,
                                ) || 'None'
                              }}</pre>
                            </div>
                          </div>
                          <!-- Thinking section -->
                          <div
                            v-if="
                              parseThinkingContent(msg.content).thinkingContent &&
                              (autoExpandThinking(msg), true)
                            "
                            class="thinking-section"
                          >
                            <div class="thinking-header" @click="() => toggleThinking(msg)">
                              <i class="i-ep-view text-sm" />
                              <span class="thinking-label">Thinking</span>
                              <el-button text size="small" class="thinking-toggle">
                                {{ msg.thinkingExpanded ? 'Collapse' : 'Expand' }}
                                <i
                                  :class="
                                    msg.thinkingExpanded
                                      ? 'i-ep-arrow-up-bold'
                                      : 'i-ep-arrow-down-bold'
                                  "
                                  class="ml-1 text-xs"
                                />
                              </el-button>
                            </div>
                            <div v-if="msg.thinkingExpanded" class="thinking-content">
                              <div
                                class="markdown-body"
                                v-html="
                                  formatMessage(parseThinkingContent(msg.content).thinkingContent)
                                "
                              />
                              <span
                                v-if="
                                  streamingMessage === msg &&
                                  !parseThinkingContent(msg.content).responseContent
                                "
                                class="typing-cursor"
                                >▊</span
                              >
                            </div>
                          </div>
                          <!-- Response section -->
                          <div
                            class="markdown-body"
                            v-if="
                              parseThinkingContent(msg.content).responseContent ||
                              !parseThinkingContent(msg.content).thinkingContent
                            "
                          >
                            <span
                              v-html="
                                formatMessage(
                                  parseThinkingContent(msg.content).responseContent || msg.content,
                                )
                              "
                            />
                            <span
                              v-if="
                                streamingMessage === msg &&
                                parseThinkingContent(msg.content).responseContent
                              "
                              class="typing-cursor"
                              >▊</span
                            >
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Loading bubble -->
                <div v-if="loading && streamingMessage === null" class="flex justify-start">
                  <div class="max-w-[80%] flex">
                    <!-- Avatar -->
                    <div
                      class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm bg-gray-100 dark:bg-gray-700 mr-3"
                    >
                      <img
                        v-if="modelIcon"
                        :src="modelIcon"
                        alt="AI"
                        class="w-full h-full object-cover rounded-full"
                        @error="(e) => handleIconError(e)"
                      />
                      <i v-else i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                    </div>
                    <!-- Loading content -->
                    <div class="flex flex-col">
                      <!-- Header with name and time -->
                      <div class="flex items-center gap-2 mb-1">
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400">
                          {{ modelName }}
                        </span>
                        <span class="text-xs text-gray-400">{{ loadingTime }}</span>
                      </div>
                      <!-- Loading dots -->
                      <div
                        class="px-4 py-3 rounded-xl shadow-md bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700"
                      >
                        <div class="flex gap-1 items-center">
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse animation-delay-200"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse animation-delay-400"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Empty state with centered input -->
            <div v-if="messages.length === 0" class="empty-state-container">
              <div class="text-center mb-6">
                <i class="i-ep-cpu text-[56px] text-primary/25 mb-3"></i>
                <h2 class="text-xl font-600 mb-2">How can I help you today?</h2>
                <p class="text-gray-500 text-sm">{{ modelName }} is ready to assist</p>
              </div>

              <!-- Example prompts -->
              <div class="grid gap-3 mb-8 max-w-[600px] mx-auto">
                <div
                  class="example-card px-4 py-3 rounded-lg border border-dashed border-gray-300 cursor-pointer transition-all hover:(border-primary shadow-sm transform scale-[1.02] bg-primary/5)"
                  @click="
                    useExample(
                      'Explain the following SQL query and point out potential performance issues: ...',
                    )
                  "
                >
                  <div class="flex items-center gap-2">
                    <i i="ep-magic-stick" class="text-primary text-lg" />
                    <span class="font-500">SQL Query Analysis</span>
                  </div>
                  <p class="text-xs text-gray-500 mt-1">
                    Explain queries and identify performance issues
                  </p>
                </div>
                <div
                  class="example-card px-4 py-3 rounded-lg border border-dashed border-gray-300 cursor-pointer transition-all hover:(border-primary shadow-sm transform scale-[1.02] bg-primary/5)"
                  @click="
                    useExample(
                      'Summarize this API specification into key points for frontend developers: ...',
                    )
                  "
                >
                  <div class="flex items-center gap-2">
                    <i i="ep-document" class="text-primary text-lg" />
                    <span class="font-500">API Documentation</span>
                  </div>
                  <p class="text-xs text-gray-500 mt-1">Summarize API specs into key points</p>
                </div>
                <div
                  class="example-card px-4 py-3 rounded-lg border border-dashed border-gray-300 cursor-pointer transition-all hover:(border-primary shadow-sm transform scale-[1.02] bg-primary/5)"
                  @click="
                    useExample(
                      'Analyze the following error logs and suggest possible root causes and debugging steps: ...',
                    )
                  "
                >
                  <div class="flex items-center gap-2">
                    <i i="ep-warning" class="text-primary text-lg" />
                    <span class="font-500">Error Analysis</span>
                  </div>
                  <p class="text-xs text-gray-500 mt-1">Analyze logs and suggest debugging steps</p>
                </div>
              </div>

              <!-- Centered input area -->
              <div class="centered-input-wrapper">
                <el-input
                  v-model="userInput"
                  type="textarea"
                  :rows="3"
                  :autosize="{ minRows: 2, maxRows: 6 }"
                  :placeholder="
                    !serviceId ? 'Please select a service first...' : 'Message AI Assistant...'
                  "
                  :disabled="!serviceId"
                  class="chat-input-centered"
                  @keydown.enter.exact.prevent="sendMessage"
                />
                <div class="input-actions-centered">
                  <div class="input-hint">Enter to send</div>
                  <el-button
                    v-if="!loading"
                    type="primary"
                    :disabled="!userInput.trim() || !serviceId"
                    class="send-button"
                    @click="sendMessage"
                  >
                    <el-icon>
                      <Promotion />
                    </el-icon>
                  </el-button>
                  <el-button v-else type="primary" class="send-button" @click="stopGeneration">
                    <el-icon>
                      <VideoPause />
                    </el-icon>
                  </el-button>
                </div>
              </div>
            </div>

            <!-- Bottom input area (shown when messages exist) -->
            <div v-if="messages.length > 0" class="input-area mt-4 pt-4">
              <div class="input-wrapper">
                <el-input
                  v-model="userInput"
                  type="textarea"
                  :rows="3"
                  :autosize="{ minRows: 2, maxRows: 6 }"
                  :placeholder="
                    !serviceId
                      ? 'Please select a service first...'
                      : 'Type your message here... (Enter to send, Shift+Enter for new line)'
                  "
                  :disabled="!serviceId"
                  class="chat-input"
                  @keydown.enter.exact.prevent="sendMessage"
                />
                <div class="input-actions">
                  <div class="input-hint">Press Enter to send, Shift+Enter for new line</div>
                  <el-button
                    v-if="!loading"
                    type="primary"
                    :disabled="!userInput.trim() || !serviceId"
                    class="send-button"
                    @click="sendMessage"
                  >
                    <el-icon>
                      <Promotion />
                    </el-icon>
                  </el-button>
                  <el-button v-else type="primary" class="send-button" @click="stopGeneration">
                    <el-icon>
                      <VideoPause />
                    </el-icon>
                  </el-button>
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </div>

      <!-- Compare mode: split screen -->
      <div v-else class="flex-1 flex flex-col" style="height: 100%; overflow: hidden">
        <!-- Split screen chat containers -->
        <div class="flex gap-4" style="flex: 1; min-height: 0; overflow: hidden">
          <!-- Primary model chat -->
          <el-card
            class="main-chat-card safe-card"
            style="flex: 1; display: flex; flex-direction: column"
            shadow="hover"
          >
            <template #header>
              <div class="flex justify-between items-center">
                <div class="flex items-center gap-3">
                  <div class="model-info flex items-center gap-2">
                    <div class="model-indicator primary"></div>
                    <el-text class="font-600 text-base">Primary: {{ modelName }}</el-text>
                  </div>
                </div>
              </div>
            </template>

            <!-- Primary chat content -->
            <div
              class="chat-content flex flex-col"
              :class="{ 'justify-center': messages.length === 0 }"
              style="flex: 1"
            >
              <div
                v-if="messages.length > 0"
                ref="primaryMessagesContainer"
                class="messages-container flex-1 overflow-y-auto pr-2"
              >
                <div class="space-y-4 py-4">
                  <div
                    v-for="(msg, index) in messages"
                    :key="index"
                    class="flex message-row"
                    :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
                  >
                    <div
                      class="max-w-[75%]"
                      :class="{
                        'order-2': msg.role === 'user',
                        'order-1': msg.role === 'assistant',
                      }"
                    >
                      <div v-if="msg.role === 'user'" class="flex justify-end mb-2">
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400 mr-3"
                          >You</span
                        >
                        <span class="text-xs text-gray-400">{{ formatTime(msg.timestamp) }}</span>
                      </div>

                      <div v-else class="flex items-center gap-2 mb-2">
                        <div
                          class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm overflow-hidden bg-gray-100 dark:bg-gray-700"
                        >
                          <img
                            v-if="modelIcon"
                            :src="modelIcon"
                            alt="AI"
                            class="w-full h-full object-cover"
                            @error="(e) => handleIconError(e)"
                          />
                          <i v-else i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                        </div>
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400">
                          {{ modelName }}
                        </span>
                        <span class="text-xs text-gray-400">{{ formatTime(msg.timestamp) }}</span>
                      </div>

                      <div
                        class="message-bubble px-4 py-3 rounded-xl shadow-sm border text-sm leading-relaxed group relative"
                        :class="
                          msg.role === 'user'
                            ? 'bg-blue-50 border-blue-200 dark:bg-blue-900/20 dark:border-blue-800'
                            : 'bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700'
                        "
                      >
                        <!-- Display primary model response -->
                        <div
                          v-if="msg.choices && msg.choices.length > 0 && msg.choices[0]"
                          class="markdown-body"
                        >
                          <span v-html="formatMessage(msg.choices[0].content || '')" />
                          <span
                            v-if="streamingMessage === msg && !msg.choices[0].finish_reason"
                            class="typing-cursor"
                            >▊</span
                          >
                        </div>
                        <!-- Fallback for user messages or direct content -->
                        <div v-else>
                          <!-- Thinking section for primary model -->
                          <div
                            v-if="
                              msg.role === 'assistant' &&
                              parseThinkingContent(msg.content || '').thinkingContent &&
                              (autoExpandThinking(msg), true)
                            "
                            class="thinking-section"
                          >
                            <div class="thinking-header" @click="() => toggleThinking(msg)">
                              <i class="i-ep-view text-sm" />
                              <span class="thinking-label">Thinking</span>
                              <el-button text size="small" class="thinking-toggle">
                                {{ msg.thinkingExpanded ? 'Collapse' : 'Expand' }}
                                <i
                                  :class="
                                    msg.thinkingExpanded
                                      ? 'i-ep-arrow-up-bold'
                                      : 'i-ep-arrow-down-bold'
                                  "
                                  class="ml-1 text-xs"
                                />
                              </el-button>
                            </div>
                            <div v-if="msg.thinkingExpanded" class="thinking-content">
                              <div
                                class="markdown-body"
                                v-html="
                                  formatMessage(
                                    parseThinkingContent(msg.content || '').thinkingContent,
                                  )
                                "
                              />
                              <span
                                v-if="
                                  streamingMessage === msg &&
                                  !parseThinkingContent(msg.content || '').responseContent
                                "
                                class="typing-cursor"
                                >▊</span
                              >
                            </div>
                          </div>
                          <!-- Response section -->
                          <div
                            class="markdown-body"
                            v-if="
                              parseThinkingContent(msg.content || '').responseContent ||
                              msg.role === 'user'
                            "
                            v-html="
                              formatMessage(
                                msg.role === 'assistant'
                                  ? parseThinkingContent(msg.content || '').responseContent ||
                                      msg.content ||
                                      ''
                                  : msg.content || '',
                              )
                            "
                          />
                          <span
                            v-if="
                              streamingMessage === msg &&
                              msg.role === 'assistant' &&
                              parseThinkingContent(msg.content || '').responseContent
                            "
                            class="typing-cursor"
                            >▊</span
                          >
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Loading indicator for primary model (hidden once streaming starts) -->
                  <div v-if="loading && !streamingMessage" class="flex justify-start">
                    <div class="flex flex-col items-start">
                      <div class="flex items-center gap-2 mb-2">
                        <div
                          class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm overflow-hidden bg-gray-100 dark:bg-gray-700 mr-3"
                        >
                          <img
                            v-if="modelIcon"
                            :src="modelIcon"
                            alt="AI"
                            class="w-full h-full object-cover"
                          />
                          <i v-else i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                        </div>
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400">
                          {{ modelName }}
                        </span>
                        <span class="text-xs text-gray-400">{{ loadingTime }}</span>
                      </div>
                      <div
                        class="message-bubble px-4 py-3 rounded-xl shadow-sm border text-sm leading-relaxed group relative bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700"
                      >
                        <div class="flex gap-1 items-center">
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse delay-100"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse delay-200"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Empty state for primary -->
              <div v-else class="flex-1 flex flex-col items-center justify-center">
                <i i="ep-chat-line-square" class="text-6xl text-gray-300 dark:text-gray-600 mb-4" />
                <p class="text-gray-500 dark:text-gray-400">
                  Start a conversation with the primary model
                </p>
              </div>
            </div>
          </el-card>

          <!-- Secondary model chat -->
          <el-card
            class="main-chat-card safe-card"
            style="flex: 1; display: flex; flex-direction: column"
            shadow="hover"
          >
            <template #header>
              <div class="flex justify-between items-center">
                <div class="flex items-center gap-3">
                  <div class="model-info flex items-center gap-2">
                    <div class="model-indicator secondary"></div>
                    <el-text class="font-600 text-base">
                      Secondary: {{ secondaryChatParams.model || 'Not Selected' }}
                    </el-text>
                  </div>
                </div>
              </div>
            </template>

            <!-- Secondary chat content -->
            <div
              class="chat-content flex flex-col"
              :class="{ 'justify-center': messages.length === 0 }"
              style="flex: 1"
            >
              <div
                v-if="messages.length > compareModeStartIndex"
                ref="secondaryMessagesContainer"
                class="messages-container flex-1 overflow-y-auto pr-2"
              >
                <div class="space-y-4 py-4">
                  <div
                    v-for="(msg, index) in messages.slice(compareModeStartIndex)"
                    :key="index + compareModeStartIndex"
                    class="flex message-row"
                    :class="msg.role === 'user' ? 'justify-end' : 'justify-start'"
                  >
                    <div
                      class="max-w-[75%]"
                      :class="{
                        'order-2': msg.role === 'user',
                        'order-1': msg.role === 'assistant',
                      }"
                    >
                      <div v-if="msg.role === 'user'" class="flex justify-end mb-2">
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400 mr-3"
                          >You</span
                        >
                        <span class="text-xs text-gray-400">{{ formatTime(msg.timestamp) }}</span>
                      </div>

                      <div v-else class="flex items-center gap-2 mb-2">
                        <div
                          class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm overflow-hidden bg-gray-100 dark:bg-gray-700"
                        >
                          <i i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                        </div>
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400">
                          {{ secondaryChatParams.model || 'Secondary' }}
                        </span>
                        <span class="text-xs text-gray-400">{{ formatTime(msg.timestamp) }}</span>
                      </div>

                      <div
                        class="message-bubble px-4 py-3 rounded-xl shadow-sm border text-sm leading-relaxed group relative"
                        :class="
                          msg.role === 'user'
                            ? 'bg-blue-50 border-blue-200 dark:bg-blue-900/20 dark:border-blue-800'
                            : 'bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700'
                        "
                      >
                        <!-- Display secondary model response -->
                        <div
                          v-if="msg.choices && msg.choices.length > 1 && msg.choices[1]"
                          class="markdown-body"
                        >
                          <span v-html="formatMessage(msg.choices[1].content || '')" />
                          <span
                            v-if="streamingMessage === msg && !msg.choices[1].finish_reason"
                            class="typing-cursor"
                            >▊</span
                          >
                        </div>
                        <!-- Fallback for user messages -->
                        <div v-else-if="msg.role === 'user'">
                          <div class="markdown-body" v-html="formatMessage(msg.content || '')" />
                        </div>
                        <!-- No response from secondary model yet -->
                        <div v-else-if="msg.role === 'assistant'">
                          <!-- Thinking section for secondary model -->
                          <div
                            v-if="
                              msg.choices &&
                              msg.choices[1] &&
                              parseThinkingContent(msg.choices[1].content).thinkingContent &&
                              (autoExpandChoiceThinking(msg.choices[1]), true)
                            "
                            class="thinking-section"
                          >
                            <div
                              class="thinking-header"
                              @click="() => toggleChoiceThinking(msg.choices![1])"
                            >
                              <i class="i-ep-view text-sm" />
                              <span class="thinking-label">Thinking</span>
                              <el-button text size="small" class="thinking-toggle">
                                {{ msg.choices![1].thinkingExpanded ? 'Collapse' : 'Expand' }}
                                <i
                                  :class="
                                    msg.choices![1].thinkingExpanded
                                      ? 'i-ep-arrow-up-bold'
                                      : 'i-ep-arrow-down-bold'
                                  "
                                  class="ml-1 text-xs"
                                />
                              </el-button>
                            </div>
                            <div v-if="msg.choices![1].thinkingExpanded" class="thinking-content">
                              <div
                                class="markdown-body"
                                v-html="
                                  formatMessage(
                                    parseThinkingContent(msg.choices![1].content).thinkingContent,
                                  )
                                "
                              />
                              <span
                                v-if="
                                  streamingMessage === msg &&
                                  !parseThinkingContent(msg.choices![1].content).responseContent
                                "
                                class="typing-cursor"
                                >▊</span
                              >
                            </div>
                          </div>
                          <!-- Response section for secondary model -->
                          <div
                            v-if="
                              msg.choices &&
                              msg.choices[1] &&
                              parseThinkingContent(msg.choices[1].content).responseContent
                            "
                            class="markdown-body"
                            v-html="
                              formatMessage(
                                parseThinkingContent(msg.choices[1].content).responseContent,
                              )
                            "
                          />
                          <span
                            v-if="
                              streamingMessage === msg &&
                              msg.choices &&
                              msg.choices[1] &&
                              parseThinkingContent(msg.choices[1].content).responseContent
                            "
                            class="typing-cursor"
                            >▊</span
                          >
                          <!-- Loading state -->
                          <div
                            v-if="!msg.choices || !msg.choices[1] || !msg.choices[1].content"
                            class="text-gray-400 italic"
                          >
                            <span v-if="streamingMessage === msg">Streaming...</span>
                            <span v-else>Waiting for secondary model response...</span>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Loading indicator for secondary model (hidden once streaming starts) -->
                  <div v-if="loading && !streamingMessage && secondaryServiceId" class="flex justify-start">
                    <div class="flex flex-col items-start">
                      <div class="flex items-center gap-2 mb-2">
                        <div
                          class="avatar-circle w-10 h-10 flex items-center justify-center rounded-full shadow-sm overflow-hidden bg-gray-100 dark:bg-gray-700 mr-3"
                        >
                          <i i="ep-cpu" class="text-gray-600 dark:text-gray-400 text-lg" />
                        </div>
                        <span class="text-xs font-500 text-gray-600 dark:text-gray-400">
                          {{ secondaryChatParams.model }}
                        </span>
                        <span class="text-xs text-gray-400">{{ loadingTime }}</span>
                      </div>
                      <div
                        class="message-bubble px-4 py-3 rounded-xl shadow-sm border text-sm leading-relaxed group relative bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700"
                      >
                        <div class="flex gap-1 items-center">
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse delay-100"
                          />
                          <span
                            class="inline-block w-2 h-2 rounded-full bg-gray-400 animate-pulse delay-200"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Empty state for secondary -->
              <div v-else class="flex-1 flex flex-col items-center justify-center">
                <i i="ep-chat-line-square" class="text-6xl text-gray-300 dark:text-gray-600 mb-4" />
                <p class="text-gray-500 dark:text-gray-400">
                  {{
                    !secondaryServiceId
                      ? 'Please select a compare service'
                      : 'Send a message to start comparing models'
                  }}
                </p>
              </div>
            </div>
          </el-card>
        </div>
        <!-- End of split screen chat containers -->

        <!-- Shared input area for compare mode -->
        <div class="input-area mt-4 pt-4">
          <!-- Control buttons -->
          <div class="flex justify-end gap-2 mb-3">
            <el-button plain size="small" @click="toggleHistory">
              <i i="ep-clock" class="mr-1" />
              History
            </el-button>
            <el-button
              plain
              size="small"
              type="danger"
              @click="clearChat"
              :disabled="messages.length === 0"
            >
              <i i="ep-delete" class="mr-1" />
              Clear Chat
            </el-button>
          </div>

          <!-- Input box -->
          <div class="input-wrapper">
            <el-input
              v-model="userInput"
              type="textarea"
              :rows="3"
              :autosize="{ minRows: 2, maxRows: 6 }"
              :placeholder="
                !secondaryServiceId
                  ? 'Please select a compare service first...'
                  : 'Type your message here... (Enter to send, Shift+Enter for new line)'
              "
              :disabled="!secondaryServiceId"
              class="chat-input"
              @keydown.enter.exact.prevent="sendMessage"
            />
            <div class="input-actions">
              <div class="input-hint">Press Enter to send, Shift+Enter for new line</div>
              <el-button
                v-if="!loading"
                type="primary"
                :disabled="!userInput.trim() || !secondaryServiceId"
                class="send-button"
                @click="sendMessage"
              >
                <el-icon>
                  <Promotion />
                </el-icon>
              </el-button>
              <el-button v-else type="danger" class="send-button" @click="stopGeneration">
                <el-icon>
                  <CloseBold />
                </el-icon>
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Conversation history drawer -->
    <el-drawer v-model="showHistory" direction="rtl" size="420px" class="history-drawer">
      <template #header>
        <div class="flex items-center gap-2">
          <i class="i-ep-clock text-lg text-primary"></i>
          <span class="font-600 text-lg">Conversation History</span>
        </div>
      </template>
      <div class="history-panel">
        <!-- Loading -->
        <div v-if="loadingSessions" class="p-2">
          <el-skeleton :rows="5" animated />
        </div>

        <!-- List -->
        <div v-else-if="sessions.length > 0" class="space-y-3">
          <div
            v-for="item in sessions"
            :key="item.id"
            class="session-card border rounded-lg p-3 cursor-pointer transition-all hover:(border-primary shadow-sm bg-primary/5) mb-2"
            :class="{ 'border-primary bg-primary/10 shadow-sm': currentSessionId === item.id }"
            @click="loadSessionDetail(item)"
          >
            <div class="flex justify-between items-center mb-2">
              <div class="flex items-center gap-2">
                <i i="ep-chat-dot-round" class="text-primary" />
                <span class="font-500 text-sm">
                  {{ item.displayName || 'Session #' + item.id }}
                </span>
              </div>
              <el-button
                size="small"
                circle
                plain
                type="danger"
                @click.stop="handleDeleteSession(item)"
              >
                <i i="ep-delete" />
              </el-button>
            </div>
            <div class="flex justify-between text-xs text-gray-500">
              <span>{{ item.modelName }}</span>
              <span>{{ formatTime(item.updateTime || item.creationTime) }}</span>
            </div>
          </div>
        </div>

        <!-- Empty -->
        <div v-else class="pt-10">
          <el-empty description="No conversation history">
            <template #image>
              <i
                i="ep-chat-line-round"
                style="font-size: 64px; color: var(--el-color-info-light-5)"
              />
            </template>
          </el-empty>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Promotion, QuestionFilled, VideoPause } from '@element-plus/icons-vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'

import {
  playgroundChatStream,
  listSessions,
  getSession,
  upsertSession,
  deleteSession,
  getModelDetail,
  getModelsList,
  getPlaygroundServices,
  type PlaygroundMessage,
  type SessionListItem,
  type PlaygroundModel,
  type PlaygroundService,
} from '@/services/playground'
import { useWorkspaceStore } from '@/stores/workspace'

// Extended message type to support multiple choices
interface MessageWithChoices extends PlaygroundMessage {
  choices?: Array<{
    index: number
    content: string
    selected?: boolean
    finish_reason?: string
    modelName?: string // For model comparison
    thinkingContent?: string // Thinking part
    responseContent?: string // Response part
    thinkingExpanded?: boolean // Whether thinking section is expanded
  }>
  selectedChoiceIndex?: number
  thinkingContent?: string // For single message
  responseContent?: string // For single message
  thinkingExpanded?: boolean // Whether thinking section is expanded
}

// ----------------- Markdown -----------------
marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderer = new marked.Renderer()
renderer.code = function ({ text, lang }: { text: string; lang?: string }) {
  if (lang && hljs.getLanguage(lang)) {
    try {
      const highlighted = hljs.highlight(text, { language: lang }).value
      return `<pre><code class="hljs language-${lang}">${highlighted}</code></pre>`
    } catch (err) {
      console.error('Highlight error:', err)
    }
  }
  const autoHighlighted = hljs.highlightAuto(text).value
  return `<pre><code class="hljs">${autoHighlighted}</code></pre>`
}
marked.use({ renderer })

const formatMessage = (content: string) => {
  try {
    return marked.parse(content) as string
  } catch (err) {
    console.error('Markdown parse error:', err)
    return content.replace(/\n/g, '<br/>')
  }
}

// Parse thinking content and answer content
interface ParsedContent {
  thinkingContent: string
  responseContent: string
}

const parseThinkingContent = (content: string): ParsedContent => {
  if (!content) {
    return {
      thinkingContent: '',
      responseContent: '',
    }
  }

  // First handle Unicode escape chars (\u003c -> <, \u003e -> >)
  const processedContent = content.replace(/\\u003c/gi, '<').replace(/\\u003e/gi, '>')

  let thinkingContent = ''
  let responseContent = ''

  // Check if contains </think> closing tag (as delimiter)
  const endThinkIndex = processedContent.search(/<\/think>/i)

  if (endThinkIndex !== -1) {
    // Found </think> tag
    // </think> All content before this is the thinking part
    let thinkPart = processedContent.substring(0, endThinkIndex)

    // Remove possible <think> opening tag
    thinkPart = thinkPart.replace(/<think>/gi, '').trim()

    thinkingContent = thinkPart

    // </think> Content after this is the answer part
    responseContent = processedContent.substring(endThinkIndex + '</think>'.length).trim()
  } else {
    // If no </think> tag, try matching complete <think>...</think> structure
    const thinkRegex = /<think>([\s\S]*?)<\/think>/gi
    const matches = processedContent.match(thinkRegex)

    if (matches && matches.length > 0) {
      thinkingContent = matches
        .map((match) => {
          return match.replace(/<\/?think>/gi, '').trim()
        })
        .filter((text) => text.length > 0)
        .join('\n\n')

      responseContent = processedContent.replace(thinkRegex, '').trim()
    } else {
      // No think tags found, treat everything as answer content
      responseContent = processedContent
    }
  }

  return {
    thinkingContent: thinkingContent.trim(),
    responseContent: responseContent.trim(),
  }
}

// ----------------- State -----------------
const route = useRoute()
const wsStore = useWorkspaceStore()
const modelId = ref((route.query.modelId as string) || '')
const modelName = ref((route.query.modelName as string) || '')
const serviceId = ref((route.query.serviceId as string) || '')
const modelIcon = ref((route.query.modelIcon as string) || '')
const systemPrompt = ref('You are a helpful assistant.')

// Services list (unified for both local infer and remote model)
const servicesList = ref<PlaygroundService[]>([])
const loadingServices = ref(false)

const messages = ref<MessageWithChoices[]>([])
const userInput = ref('')
const loading = ref(false)
const secondaryLoading = ref(false)
const loadingTime = ref('')
const messagesContainer = ref<HTMLElement>()
const primaryMessagesContainer = ref<HTMLElement>()
const secondaryMessagesContainer = ref<HTMLElement>()
const savingSession = ref(false)

// Streaming message state
const streamingMessage = ref<MessageWithChoices | null>(null)

// Debug mode
const debugMode = ref(false)

// Abort controller for stopping streaming
const abortController = ref<AbortController | null>(null)

// Model list for selection (kept for compatibility)
const modelsList = ref<PlaygroundModel[]>([])
const loadingModels = ref(false)
const compareMode = ref(false)
const secondaryModelId = ref('')
const secondaryServiceId = ref('')
const compareModeStartIndex = ref(0) // Track when compare mode was enabled

// Max tokens limit from model details
const maxTokensLimit = ref(32768)
const secondaryMaxTokensLimit = ref(32768)

// Chat parameters
const chatParams = ref({
  model: modelName.value,
  maxTokens: 8192,
  temperature: 0.7,
  topP: 0.7,
  frequencyPenalty: 0.0,
  // enableThinking: false,
  // thinkingBudget: 4096,
})

// Secondary model chat parameters (for comparison)
const secondaryChatParams = ref({
  model: '',
  maxTokens: 8192,
  temperature: 0.7,
  topP: 0.7,
  frequencyPenalty: 0.0,
  // enableThinking: false,
  // thinkingBudget: 4096,
})

// Fetch available models
const fetchModelsList = async () => {
  loadingModels.value = true
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const params: any = {}
    if (wsStore.currentWorkspaceId) params.workspace = wsStore.currentWorkspaceId

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res: any = await getModelsList(params)
    modelsList.value = res.items || []
  } catch (error) {
    console.error('Failed to fetch models:', error)
  } finally {
    loadingModels.value = false
  }
}

// Fetch services list (unified for both local infer and remote model)
const fetchServicesList = async () => {
  loadingServices.value = true
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const params: any = {}
    if (wsStore.currentWorkspaceId) params.workspace = wsStore.currentWorkspaceId

    const res = (await getPlaygroundServices(params)) as unknown as { items: PlaygroundService[] }
    servicesList.value = res?.items || []
  } catch (error) {
    console.error('Failed to fetch services:', error)
    ElMessage.error('Failed to load services')
  } finally {
    loadingServices.value = false
  }
}

// Handle service selection change
const handleServiceChange = (selectedServiceId: string) => {
  const selectedService = servicesList.value.find((s) => s.id === selectedServiceId)
  if (selectedService) {
    serviceId.value = selectedServiceId
    // Prefer service.modelName, fall back to displayName, keep original if both empty
    const newModelName = selectedService.modelName || selectedService.displayName
    if (newModelName) {
      modelName.value = newModelName
      chatParams.value.model = modelName.value
    }
  }
}

// Get phase type for display
const getPhaseType = (phase: string) => {
  const typeMap: Record<string, string> = {
    Running: 'success',
    Pending: 'warning',
    Failed: 'danger',
    Succeeded: 'success',
  }
  return typeMap[phase] || 'info'
}

// Handle secondary service change (for comparison)
const handleSecondaryServiceChange = async (selectedSvcId: string) => {
  const selectedService = servicesList.value.find((s) => s.id === selectedSvcId)
  if (selectedService) {
    secondaryServiceId.value = selectedSvcId
    secondaryModelId.value = selectedSvcId
    secondaryChatParams.value.model =
      selectedService.modelName || selectedService.displayName || 'Unknown Model'
  }
}

const currentSessionId = ref<number | null>(null)

const showHistory = ref(false)
const sessions = ref<SessionListItem[]>([])
const loadingSessions = ref(false)

// ----------------- Utils -----------------
const scrollToBottom = () => {
  if (compareMode.value) {
    // In compare mode, scroll both containers
    if (primaryMessagesContainer.value) {
      primaryMessagesContainer.value.scrollTop = primaryMessagesContainer.value.scrollHeight
    }
    if (secondaryMessagesContainer.value) {
      secondaryMessagesContainer.value.scrollTop = secondaryMessagesContainer.value.scrollHeight
    }
  } else {
    // In single mode, scroll the main container
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  }
}

const formatTime = (ts?: string) => {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleString()
}

const formatMsgTime = (ts?: string) => {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleTimeString()
}

const buildMessagesForChat = (msgs: MessageWithChoices[], system?: string): PlaygroundMessage[] => {
  const list: PlaygroundMessage[] = []
  if (system) {
    list.push({ role: 'system', content: system })
  }
  list.push(
    ...msgs.map((m) => ({
      role: m.role,
      content: m.content,
    })),
  )
  return list
}

const useExample = (example: string) => {
  userInput.value = example
  // Focus the input after setting example text
  nextTick(() => {
    const textareas = document.querySelectorAll('.el-textarea__inner')
    const targetTextarea =
      messages.value.length === 0 ? textareas[0] : textareas[textareas.length - 1]
    if (targetTextarea) {
      ;(targetTextarea as HTMLTextAreaElement).focus()
    }
  })
}

// ----------------- Stop message generation -----------------
const stopGeneration = () => {
  if (abortController.value) {
    abortController.value.abort()
    abortController.value = null
  }
  loading.value = false
  streamingMessage.value = null
}

// ----------------- Send message -----------------
const sendMessage = async () => {
  const query = userInput.value.trim()
  if (!query || loading.value) return

  // Check if service is selected
  if (!serviceId.value) {
    ElMessage.warning('Please select a service first')
    return
  }

  const now = new Date().toISOString()

  // 1. append user message locally
  const userMsg: PlaygroundMessage = {
    role: 'user',
    content: query,
    timestamp: now,
  }
  messages.value.push(userMsg)
  userInput.value = ''
  await nextTick()
  scrollToBottom()

  loading.value = true
  loadingTime.value = new Date().toLocaleTimeString()

  // Create new AbortController for this request
  abortController.value = new AbortController()

  try {
    const timestamp = new Date().toISOString()

    // Check if compare mode is enabled and secondary model is selected
    if (compareMode.value && secondaryModelId.value && secondaryServiceId.value) {
      // Build chat messages BEFORE pushing assistant placeholder to avoid
      // sending a trailing empty assistant message to the backend
      const chatMessages = buildMessagesForChat(messages.value, systemPrompt.value)

      const message: MessageWithChoices = {
        role: 'assistant',
        content: '',
        timestamp,
        choices: [
          {
            index: 0,
            content: '',
            modelName: modelName.value,
            selected: true,
          },
          {
            index: 1,
            content: '',
            modelName: secondaryChatParams.value.model,
            selected: false,
          },
        ],
        selectedChoiceIndex: undefined,
      }
      messages.value.push(message)
      const msgIndex = messages.value.length - 1
      streamingMessage.value = messages.value[msgIndex]

      await nextTick()
      scrollToBottom()

      // Use separate AbortControllers so aborting one stream doesn't cancel the other
      const primaryAbort = new AbortController()
      const secondaryAbort = new AbortController()
      const parentSignal = abortController.value?.signal
      if (parentSignal) {
        parentSignal.addEventListener('abort', () => {
          primaryAbort.abort()
          secondaryAbort.abort()
        })
      }

      const STREAM_TIMEOUT = 120_000
      const setChoiceTimeout = (
        abort: AbortController,
        choiceIdx: number,
        label: string,
      ) => {
        const timer = setTimeout(() => {
          abort.abort()
          const reactiveMsg = messages.value[msgIndex]
          if (reactiveMsg?.choices?.[choiceIdx] && !reactiveMsg.choices[choiceIdx].content) {
            reactiveMsg.choices[choiceIdx].content = `Error: ${label} response timed out`
            reactiveMsg.choices[choiceIdx].finish_reason = 'error'
          }
        }, STREAM_TIMEOUT)
        return timer
      }

      const primaryTimer = setChoiceTimeout(primaryAbort, 0, 'Primary model')
      const secondaryTimer = setChoiceTimeout(secondaryAbort, 1, 'Compare model')

      // Stream both models simultaneously
      await Promise.allSettled([
        // Primary model stream
        playgroundChatStream(
          {
            serviceId: serviceId.value,
            messages: chatMessages,
            modelName: modelName.value,
            temperature: chatParams.value.temperature,
            topP: chatParams.value.topP,
            maxTokens: chatParams.value.maxTokens,
            frequencyPenalty: chatParams.value.frequencyPenalty,
            presencePenalty: 0,
          },
          (content: string) => {
            const reactiveMsg = messages.value[msgIndex]
            if (reactiveMsg?.choices?.[0]) {
              reactiveMsg.choices[0].content += content
              nextTick(() => scrollToBottom())
            }
          },
          (error: unknown) => {
            console.error('Primary model streaming error:', error)
            const reactiveMsg = messages.value[msgIndex]
            if (reactiveMsg?.choices?.[0]) {
              reactiveMsg.choices[0].content = 'Error: Failed to get response from primary model'
              reactiveMsg.choices[0].finish_reason = 'error'
            }
          },
          () => clearTimeout(primaryTimer),
          primaryAbort.signal,
        ),
        // Secondary model stream
        playgroundChatStream(
          {
            serviceId: secondaryServiceId.value,
            messages: chatMessages,
            modelName: secondaryChatParams.value.model,
            temperature: secondaryChatParams.value.temperature,
            topP: secondaryChatParams.value.topP,
            maxTokens: secondaryChatParams.value.maxTokens,
            frequencyPenalty: secondaryChatParams.value.frequencyPenalty,
            presencePenalty: 0,
          },
          (content: string) => {
            const reactiveMsg = messages.value[msgIndex]
            if (reactiveMsg?.choices?.[1]) {
              reactiveMsg.choices[1].content += content
              nextTick(() => scrollToBottom())
            }
          },
          (error: unknown) => {
            console.error('Secondary model streaming error:', error)
            const reactiveMsg = messages.value[msgIndex]
            if (reactiveMsg?.choices?.[1]) {
              reactiveMsg.choices[1].content = 'Error: Failed to get response from compare model'
              reactiveMsg.choices[1].finish_reason = 'error'
            }
          },
          () => clearTimeout(secondaryTimer),
          secondaryAbort.signal,
        ),
      ])

      clearTimeout(primaryTimer)
      clearTimeout(secondaryTimer)
      streamingMessage.value = null
    } else {
      // Single model mode with streaming
      // Build chat messages BEFORE pushing assistant placeholder
      const chatMessages = buildMessagesForChat(messages.value, systemPrompt.value)

      const message: MessageWithChoices = {
        role: 'assistant',
        content: '',
        timestamp,
      }
      messages.value.push(message)
      streamingMessage.value = messages.value[messages.value.length - 1]

      await nextTick()
      scrollToBottom()

      // Use streaming API
      await playgroundChatStream(
        {
          serviceId: serviceId.value,
          messages: chatMessages,
          modelName: modelName.value,
          temperature: chatParams.value.temperature,
          topP: chatParams.value.topP,
          maxTokens: chatParams.value.maxTokens,
          frequencyPenalty: chatParams.value.frequencyPenalty,
          presencePenalty: 0,
        },
        // onMessage callback
        (content: string) => {
          if (streamingMessage.value) {
            streamingMessage.value.content += content
            nextTick(() => scrollToBottom())
          }
        },
        // onError callback
        (error: unknown) => {
          console.error('Streaming error:', error)
          if (streamingMessage.value) {
            streamingMessage.value.content = 'Error: Failed to receive response from the model.'
          }
        },
        // onFinish callback
        () => {
          streamingMessage.value = null
        },
        abortController.value?.signal,
      )
    }

    await nextTick()
    scrollToBottom()
  } catch (err) {
    console.error('sendMessage error:', err)

    interface ErrorResponse {
      response?: {
        data?: {
          error?: string
        }
      }
      message?: string
    }
    const errTyped = err as ErrorResponse
    const raw = errTyped?.response?.data?.error || errTyped?.message || ''

    let uiMsg = raw || 'Request failed'

    if (raw.includes('x509: certificate signed by unknown authority')) {
      uiMsg =
        'The AI backend service is temporarily unavailable due to a certificate configuration issue. The backend team has been notified. Please try again later.'
    }

    ElMessage.error(uiMsg)
  } finally {
    loading.value = false
    abortController.value = null
    streamingMessage.value = null
  }
}

// ----------------- History -----------------
const fetchSessions = async () => {
  loadingSessions.value = true
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const data: any = await listSessions({
      limit: 50,
      offset: 0,
      modelName: modelName.value,
    })
    sessions.value = data.items || []
  } catch (err) {
    console.error('listSessions error:', err)
    ElMessage.error((err as Error)?.message || 'Failed to load sessions')
  } finally {
    loadingSessions.value = false
  }
}

const toggleHistory = () => {
  showHistory.value = !showHistory.value
  if (showHistory.value) {
    fetchSessions()
  }
}

const loadSessionDetail = async (item: SessionListItem) => {
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const detail: any = await getSession(item.id)
    // Parse messages if it's a JSON string
    let messagesData = detail.messages || []
    if (typeof messagesData === 'string') {
      try {
        messagesData = JSON.parse(messagesData)
      } catch (e) {
        console.error('Failed to parse messages JSON:', e)
        messagesData = []
      }
    }

    messages.value = messagesData.map(
      (m: PlaygroundMessage) =>
        ({
          ...m,
          timestamp: m.timestamp || detail.updatedAt,
        }) as MessageWithChoices,
    )
    // Restore system prompt if available
    if (detail.systemPrompt) {
      systemPrompt.value = detail.systemPrompt
    }
    currentSessionId.value = detail.id
    await nextTick()
    scrollToBottom()
    ElMessage.success('Conversation loaded')
  } catch (err) {
    console.error('getSession error:', err)
    ElMessage.error((err as Error)?.message || 'Failed to load conversation')
  }
}

const handleDeleteSession = async (item: SessionListItem) => {
  try {
    await ElMessageBox.confirm(
      'Are you sure you want to delete this conversation? This action cannot be undone.',
      'Confirm Deletion',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )
    await deleteSession(item.id)
    ElMessage.success('Conversation deleted')

    if (currentSessionId.value === item.id) {
      clearChat()
    }

    fetchSessions()
  } catch (err) {
    if (err !== 'cancel') {
      console.error('deleteSession error:', err)
      ElMessage.error((err as Error)?.message || 'Failed to delete conversation')
    }
  }
}

// ----------------- Other actions -----------------
const clearChat = () => {
  messages.value = []
  currentSessionId.value = null
}

const saveSession = async () => {
  if (messages.value.length === 0) return

  savingSession.value = true
  try {
    const firstUser = messages.value.find((m) => m.role === 'user')
    const displayName = firstUser?.content?.slice(0, 30) || 'New Conversation'

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const sessionResp: any = await upsertSession({
      id: currentSessionId.value ?? 0,
      modelName: modelName.value,
      displayName,
      systemPrompt: systemPrompt.value,
      messages: messages.value,
    })
    currentSessionId.value = sessionResp.id

    ElMessage.success('Session saved successfully')

    if (showHistory.value) {
      await fetchSessions()
    }
  } catch (error) {
    console.error('Failed to save session:', error)
    ElMessage.error('Failed to save session')
  } finally {
    savingSession.value = false
  }
}

const selectChoice = (message: MessageWithChoices, choiceIndex: number) => {
  if (message.choices && message.choices[choiceIndex]) {
    message.selectedChoiceIndex = choiceIndex
    message.content = message.choices[choiceIndex].content
    message.choices?.forEach((c, i) => {
      c.selected = i === choiceIndex
    })
  }
}

// Toggle thinking section expand/collapse state
const toggleThinking = (message: MessageWithChoices) => {
  message.thinkingExpanded = !message.thinkingExpanded
}

// Auto-expand thinking section (during streaming render)
const autoExpandThinking = (message: MessageWithChoices) => {
  if (message.thinkingExpanded === undefined) {
    message.thinkingExpanded = true
  }
}

// Toggle expand/collapse state of choice thinking section
const toggleChoiceThinking = (choice: {
  index: number
  content: string
  selected?: boolean
  finish_reason?: string
  modelName?: string
  thinkingContent?: string
  responseContent?: string
  thinkingExpanded?: boolean
}) => {
  choice.thinkingExpanded = !choice.thinkingExpanded
}

// Auto-expand choice's thinking section
const autoExpandChoiceThinking = (choice: {
  index: number
  content: string
  selected?: boolean
  finish_reason?: string
  modelName?: string
  thinkingContent?: string
  responseContent?: string
  thinkingExpanded?: boolean
}) => {
  if (choice.thinkingExpanded === undefined) {
    choice.thinkingExpanded = true
  }
}

// ----------------- Model Details -----------------
// Helper function to update maxTokens settings
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const updateMaxTokensFromDetail = (detail: any, isSecondary: boolean = false) => {
  if (!detail?.maxTokens) return

  const targetMaxTokensLimit = isSecondary ? secondaryMaxTokensLimit : maxTokensLimit
  const targetChatParams = isSecondary ? secondaryChatParams : chatParams

  targetMaxTokensLimit.value = detail.maxTokens
  // If max is less than 8192, set default to half of max
  if (detail.maxTokens < 8192) {
    targetChatParams.value.maxTokens = Math.floor(detail.maxTokens / 2)
  } else {
    // Reset to default
    targetChatParams.value.maxTokens = 8192
  }
}

const fetchModelDetails = async () => {
  if (!modelId.value) return

  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const detail: any = await getModelDetail(modelId.value)
    if (detail) {
      // Set icon
      if (detail.icon) {
        modelIcon.value = detail.icon
      }
      // Set maxTokens limit and default value
      updateMaxTokensFromDetail(detail, false)
    }
  } catch (error) {
    console.error('Failed to fetch model details:', error)
  }
}

// Handle icon error
const handleIconError = (e: Event) => {
  const target = e.target as HTMLImageElement
  target.style.display = 'none'
  modelIcon.value = ''
}

// Watch for compare mode changes
watch(compareMode, (newValue) => {
  if (newValue) {
    // When compare mode is enabled, record the current message count
    // Secondary chat will only show messages from this point forward
    compareModeStartIndex.value = messages.value.length
  } else {
    // When compare mode is disabled, reset the index
    compareModeStartIndex.value = 0
    // Clear secondary model selection
    secondaryModelId.value = ''
    secondaryServiceId.value = ''
    secondaryChatParams.value.model = ''
  }
})

// Watch for workspace changes
watch(
  () => wsStore.currentWorkspaceId,
  async (newWorkspaceId, oldWorkspaceId) => {
    if (newWorkspaceId !== oldWorkspaceId) {
      // Refresh services and models list when workspace changes
      await fetchServicesList()
      await fetchModelsList()
    }
  },
)

// ----------------- Lifecycle -----------------
onMounted(async () => {
  // Fetch services list (unified for both local and remote)
  await fetchServicesList()

  // Sync modelName with selected service
  if (serviceId.value) {
    const selectedService = servicesList.value.find((s) => s.id === serviceId.value)
    if (selectedService) {
      // Prefer service.modelName, fall back to displayName, keep route param modelName if both empty
      const newModelName = selectedService.modelName || selectedService.displayName
      if (newModelName) {
        modelName.value = newModelName
        chatParams.value.model = modelName.value
      }
    }
  }

  // Fetch models list for comparison mode
  await fetchModelsList()

  // If modelId is provided, fetch model details for icon
  if (modelId.value) {
    fetchModelDetails()
  }
})
</script>

<style scoped>
/* Page container with ambient effects */
.agent-page-container {
  height: calc(100vh - 50px);
  position: relative;
  background: linear-gradient(135deg, #f5f7fa 0%, #f0f2f5 50%, #e9ecef 100%);
  padding: 0;
  margin: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  align-items: stretch;
}

.agent-page-container::before {
  content: '';
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background:
    radial-gradient(circle at 15% 50%, rgba(120, 119, 198, 0.03) 0%, transparent 50%),
    radial-gradient(circle at 85% 30%, rgba(255, 119, 168, 0.03) 0%, transparent 50%),
    radial-gradient(circle at 50% 90%, rgba(99, 102, 241, 0.02) 0%, transparent 50%);
  pointer-events: none;
  z-index: 0;
}

.dark .agent-page-container {
  background: linear-gradient(135deg, #1a1a1e 0%, #18181c 50%, #1c1a20 100%);
}

.dark .agent-page-container::before {
  background:
    radial-gradient(circle at 15% 50%, rgba(120, 119, 198, 0.08) 0%, transparent 50%),
    radial-gradient(circle at 85% 30%, rgba(255, 119, 168, 0.08) 0%, transparent 50%),
    radial-gradient(circle at 50% 90%, rgba(99, 102, 241, 0.06) 0%, transparent 50%);
}

/* Parameters panel */
.parameters-panel {
  border-radius: 8px;
  border: 1px solid #e4e7ed;
  overflow: hidden;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.04);
}

.parameters-panel :deep(.el-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 20px;
  overflow: hidden;
}

.dark .parameters-panel {
  border-color: #2b2b2b;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.params-content {
  padding: 16px;
}

.param-item {
  margin-bottom: 24px;
}

.param-item:last-child {
  margin-bottom: 0;
}

.param-label {
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 500;
  color: #606266;
  margin-bottom: 8px;
}

.dark .param-label {
  color: #ccc;
}

.system-prompt-input :deep(.el-textarea__inner) {
  font-family:
    'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  background-color: #f5f7fa;
}

.dark .system-prompt-input :deep(.el-textarea__inner) {
  background-color: #1d1e1f;
}

.w-full {
  width: 100%;
}

.ml-1 {
  margin-left: 4px;
}

.flex-1 {
  flex: 1;
}

.gap-2 {
  gap: 8px;
}

/* Main chat card */
.main-chat-card {
  border-radius: 12px;
  border: 1px solid rgba(228, 231, 237, 0.8);
  overflow: hidden;
  box-shadow:
    0 4px 12px rgba(0, 0, 0, 0.04),
    0 1px 3px rgba(0, 0, 0, 0.02);
  position: relative;
  background: linear-gradient(145deg, #ffffff 0%, #fafbfc 100%);
  height: 100%;
  display: flex;
  flex-direction: column;
}

.main-chat-card :deep(.el-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.main-chat-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background:
    radial-gradient(circle at 20% 80%, rgba(99, 102, 241, 0.02) 0%, transparent 50%),
    radial-gradient(circle at 80% 20%, rgba(244, 114, 182, 0.02) 0%, transparent 50%),
    radial-gradient(circle at 50% 50%, rgba(59, 130, 246, 0.01) 0%, transparent 70%);
  pointer-events: none;
  z-index: 0;
}

.dark .main-chat-card {
  border-color: rgba(43, 43, 43, 0.6);
  box-shadow:
    0 8px 24px rgba(0, 0, 0, 0.4),
    0 2px 8px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.02);
  background: linear-gradient(145deg, rgba(26, 26, 30, 0.98) 0%, rgba(24, 24, 28, 0.95) 100%);
}

.dark .main-chat-card::before {
  background:
    radial-gradient(circle at 20% 80%, rgba(99, 102, 241, 0.06) 0%, transparent 50%),
    radial-gradient(circle at 80% 20%, rgba(244, 114, 182, 0.06) 0%, transparent 50%),
    radial-gradient(circle at 50% 50%, rgba(59, 130, 246, 0.03) 0%, transparent 70%);
}

.main-chat-card :deep(.el-card__header) {
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98) 0%, rgba(250, 251, 252, 0.95) 100%);
  border-bottom: 1px solid rgba(235, 238, 245, 0.6);
  padding: 16px 24px;
  position: relative;
  z-index: 1;
  backdrop-filter: blur(10px);
}

.dark .main-chat-card :deep(.el-card__header) {
  background: linear-gradient(180deg, rgba(28, 28, 32, 0.98) 0%, rgba(26, 26, 30, 0.95) 100%);
  border-bottom: 1px solid rgba(43, 43, 43, 0.5);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2);
}

/* Model indicator */
.model-info {
  position: relative;
}

.model-indicator {
  width: 8px;
  height: 8px;
  background: #52c41a;
  border-radius: 50%;
  position: relative;
  animation: pulse 2s infinite;

  &.primary {
    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
    box-shadow: 0 0 12px rgba(102, 126, 234, 0.5);
  }

  &.secondary {
    background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
    box-shadow: 0 0 12px rgba(240, 147, 251, 0.5);
  }
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(82, 196, 26, 0.4);
  }
  50% {
    box-shadow: 0 0 0 8px rgba(82, 196, 26, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(82, 196, 26, 0);
  }
}

.session-tag {
  font-size: 12px;
  padding: 2px 10px;
  height: 24px;
}

.header-btn {
  height: 36px;
  padding: 0 20px;
  font-size: 14px;
  border-radius: 6px;
}

/* Chat content */
.chat-content {
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.95) 0%, rgba(250, 250, 252, 0.9) 100%);
  backdrop-filter: blur(20px);
  transition: all 0.3s ease;
  border-radius: 12px;
  box-shadow:
    inset 0 2px 4px rgba(0, 0, 0, 0.04),
    0 8px 32px rgba(0, 0, 0, 0.06);
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  flex: 1;
  padding: 0;
}

.chat-content::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 200px;
  background: radial-gradient(ellipse at top, rgba(147, 197, 253, 0.1) 0%, transparent 70%);
  pointer-events: none;
}

.dark .chat-content {
  background: linear-gradient(
    135deg,
    rgba(30, 30, 35, 0.95) 0%,
    rgba(25, 25, 30, 0.9) 50%,
    rgba(35, 30, 40, 0.85) 100%
  );
  box-shadow:
    inset 0 2px 6px rgba(0, 0, 0, 0.3),
    inset 0 -1px 2px rgba(255, 255, 255, 0.03),
    0 10px 40px rgba(0, 0, 0, 0.5);
}

.dark .chat-content::before {
  background: radial-gradient(ellipse at top, rgba(147, 197, 253, 0.03) 0%, transparent 70%);
}

.dark .chat-content::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 300px;
  background: radial-gradient(
    ellipse at bottom right,
    rgba(139, 92, 246, 0.03) 0%,
    transparent 70%
  );
  pointer-events: none;
}

.messages-container {
  padding: 24px;
  background: transparent;
  position: relative;
  z-index: 1;
  flex: 1;
  overflow-y: auto;
}

/* Example cards */
.example-card {
  background: #ffffff;
  border-color: #e4e7ed;
}

.dark .example-card {
  background: #1f1f1f;
  border-color: #333;
}

/* Message rows */
.message-row {
  margin-bottom: 1.5rem;
}

/* Message bubbles */
.message-bubble {
  word-wrap: break-word;
  box-shadow:
    0 3px 10px rgba(0, 0, 0, 0.08),
    0 1px 3px rgba(0, 0, 0, 0.04),
    inset 0 1px 0 rgba(255, 255, 255, 0.5);
  transition: all 0.2s ease;
  position: relative;
  min-width: 150px;
  backdrop-filter: blur(15px);
  background: linear-gradient(145deg, rgba(255, 255, 255, 0.95), rgba(255, 255, 255, 0.9));
}

.message-bubble:hover {
  box-shadow:
    0 5px 15px rgba(0, 0, 0, 0.12),
    0 2px 5px rgba(0, 0, 0, 0.06),
    inset 0 1px 0 rgba(255, 255, 255, 0.7);
  transform: translateY(-1px);
}

/* Dark mode enhancements */
html.dark .message-bubble {
  box-shadow:
    0 4px 12px rgba(0, 0, 0, 0.5),
    0 1px 3px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.05);
  background: linear-gradient(145deg, rgba(42, 42, 46, 0.95), rgba(38, 38, 42, 0.9));
}

html.dark .message-bubble:hover {
  box-shadow:
    0 6px 16px rgba(0, 0, 0, 0.6),
    0 2px 6px rgba(0, 0, 0, 0.4),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

/* Animation delays for loading dots */
.animation-delay-200 {
  animation-delay: 200ms;
}

.animation-delay-400 {
  animation-delay: 400ms;
}

/* Avatar circle */
.avatar-circle {
  flex-shrink: 0;
}

.avatar-circle img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Input area */
.input-area {
  border-top: 1px solid #ebeef5;
  background: #fafbfc;
  padding: 16px 24px;
  margin: 0;
  flex-shrink: 0;
}

.dark .input-area {
  background: #0d0d0d;
  border-top: 1px solid #2b2b2b;
}

.input-wrapper {
  position: relative;
}

.chat-input :deep(.el-textarea__inner),
.chat-input-centered :deep(.el-textarea__inner) {
  border-radius: 8px;
  font-size: 14px;
  line-height: 1.6;
  padding: 12px 120px 12px 16px;
  border: 1px solid #dcdfe6;
  background: #ffffff;
  transition: all 0.2s ease;
  resize: none;
}

.dark .chat-input :deep(.el-textarea__inner),
.dark .chat-input-centered :deep(.el-textarea__inner) {
  background: #1a1a1a;
  border-color: #333;
}

.chat-input :deep(.el-textarea__inner:focus) {
  border-color: var(--el-color-primary);
  box-shadow:
    0 0 0 2px rgba(64, 158, 255, 0.2),
    0 0 12px rgba(64, 158, 255, 0.1);
  background: #ffffff;
}

.dark .chat-input :deep(.el-textarea__inner:focus) {
  background: #1a1a1a;
  box-shadow:
    0 0 0 2px rgba(64, 158, 255, 0.15),
    0 0 12px rgba(64, 158, 255, 0.08);
}

.input-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-hint {
  font-size: 12px;
  color: #909399;
  padding: 0 8px;
  user-select: none;
}

.send-button {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 500;
  transition: all 0.2s ease;
}

.send-button:hover:not(:disabled) {
  transform: scale(1.05);
}

.send-button :deep(.el-icon) {
  font-size: 20px;
}

/* Empty state and centered input */
.empty-state-container {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  flex: 1;
  padding: 40px 20px;
  animation: fadeIn 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

.centered-input-wrapper {
  position: relative;
  width: 100%;
  max-width: 680px;
  margin: 0 auto;
  transition: all 0.3s ease;
}

.chat-input-centered :deep(.el-textarea__inner) {
  padding: 16px 60px 16px 20px;
  font-size: 15px;
  border: 2px solid transparent;
  background: #ffffff;
  box-shadow:
    0 0 0 1px #e4e7ed,
    0 2px 12px rgba(0, 0, 0, 0.08);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.dark .chat-input-centered :deep(.el-textarea__inner) {
  background: #1a1a1a;
  box-shadow:
    0 0 0 1px #333,
    0 2px 12px rgba(0, 0, 0, 0.3);
}

.chat-input-centered :deep(.el-textarea__inner:focus) {
  border-color: var(--el-color-primary);
  box-shadow:
    0 0 0 2px rgba(64, 158, 255, 0.25),
    0 0 25px rgba(64, 158, 255, 0.15);
  transform: translateY(-1px);
}

.dark .chat-input-centered :deep(.el-textarea__inner:focus) {
  box-shadow:
    0 0 0 2px rgba(64, 158, 255, 0.2),
    0 0 25px rgba(64, 158, 255, 0.1);
}

.input-actions-centered {
  position: absolute;
  right: 12px;
  bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* Transition for message container appearance */
.messages-container {
  animation: slideDown 0.3s ease;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* History drawer */
.history-drawer :deep(.el-drawer__header) {
  border-bottom: 1px solid #ebeef5;
  margin-bottom: 0;
  background: #fafbfc;
  padding: 20px 24px;
}

.dark .history-drawer :deep(.el-drawer__header) {
  background: #1a1a1a;
  border-bottom: 1px solid #2b2b2b;
}

.history-panel {
  padding: 20px;
  height: 100%;
  overflow-y: auto;
  background: #f5f7fa;
}

.dark .history-panel {
  background: #0d0d0d;
}

.session-card {
  background: #ffffff;
  border: 1px solid #e4e7ed;
  transition: all 0.2s ease;
  margin-bottom: 8px;
}

.session-card:last-child {
  margin-bottom: 0;
}

.dark .session-card {
  background: #1a1a1a;
  border-color: #2b2b2b;
}

.session-card:hover {
  transform: translateX(-2px);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.dark .session-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

/* Markdown styles */
.markdown-body :deep(pre) {
  padding: 12px 16px;
  border-radius: 6px;
  background: #2d2d2d;
  font-size: 13px;
  overflow: auto;
  margin: 12px 0;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.markdown-body :deep(code) {
  font-family:
    'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
    'Courier New', monospace;
  font-size: 0.9em;
  padding: 2px 6px;
  border-radius: 3px;
  background: rgba(0, 0, 0, 0.06);
  color: #c7254e;
}

.dark .markdown-body :deep(code) {
  background: rgba(255, 255, 255, 0.1);
  color: #f92672;
}

.markdown-body :deep(p) {
  margin: 8px 0;
  line-height: 1.6;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  padding-left: 24px;
  margin: 8px 0;
}

.markdown-body :deep(li) {
  margin: 4px 0;
}

.markdown-body :deep(blockquote) {
  border-left: 4px solid var(--el-color-primary);
  padding-left: 16px;
  margin: 12px 0;
  color: var(--el-text-color-regular);
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  margin: 16px 0 8px;
  font-weight: 600;
}

.markdown-body :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 12px 0;
}

.markdown-body :deep(table th),
.markdown-body :deep(table td) {
  border: 1px solid var(--el-border-color);
  padding: 8px 12px;
}

.markdown-body :deep(table th) {
  background: var(--el-fill-color-light);
  font-weight: 600;
}

/* Choices container styles */
.choices-container {
  margin-top: 8px;
  padding: 12px;
  background: rgba(64, 158, 255, 0.03);
  border-radius: 8px;
  border: 1px solid rgba(64, 158, 255, 0.1);
}

.dark .choices-container {
  background: rgba(64, 158, 255, 0.05);
  border-color: rgba(64, 158, 255, 0.15);
}

.choices-header {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
  font-size: 13px;
}

.choices-tabs {
  margin-bottom: 16px;
}

.choice-content {
  padding: 12px;
  background: var(--el-bg-color);
  border-radius: 6px;
  border: 1px solid var(--el-border-color-lighter);
}

.dark .choice-content {
  background: #1a1a1a;
  border-color: #2b2b2b;
}

/* Radio button group styling */
.choices-tabs :deep(.el-radio-button__inner) {
  padding: 6px 16px;
  font-size: 13px;
}

.choices-tabs :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background-color: var(--el-color-primary);
  border-color: var(--el-color-primary);
  box-shadow: 0 0 4px rgba(64, 158, 255, 0.3);
}

/* Model comparison styles */
.param-item .el-divider {
  margin: 16px 0;
}

.compare-mode-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  border-radius: 4px;
  font-size: 12px;
  margin-left: 8px;
}

/* Side-by-side comparison */
.compare-container {
  width: 100%;
}

.compare-header {
  display: flex;
  align-items: center;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.compare-responses {
  display: flex;
  gap: 16px;
  margin-top: 12px;
  position: relative;
}

.compare-responses::after {
  content: '';
  position: absolute;
  left: calc(50% - 0.5px);
  top: 0;
  bottom: 0;
  width: 1px;
  background: var(--el-border-color-lighter);
}

.compare-response-item {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
  background: var(--el-bg-color);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.05);
  transition: box-shadow 0.2s;
}

.compare-response-item:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.response-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-bottom: 1px solid var(--el-border-color-lighter);
  position: relative;
}

/* Color indicators for different models */
.compare-response-item:first-child .response-header::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--el-color-primary);
}

.compare-response-item:last-child .response-header::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--el-color-success);
}

.model-name {
  font-weight: 500;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.response-content {
  padding: 12px;
  max-height: 600px;
  overflow-y: auto;
}

/* Dark mode adjustments */
.dark .compare-response-item {
  background: #1a1a1a;
  border-color: #333;
}

.dark .response-header {
  background: #252525;
  border-color: #333;
}

/* Scrollbar for compare responses */
.response-content::-webkit-scrollbar {
  width: 6px;
}

.response-content::-webkit-scrollbar-track {
  background: var(--el-fill-color-lighter);
  border-radius: 3px;
}

.response-content::-webkit-scrollbar-thumb {
  background: var(--el-border-color);
  border-radius: 3px;
}

.response-content::-webkit-scrollbar-thumb:hover {
  background: var(--el-border-color-darker);
}

/* Animations */
@keyframes fadeInDown {
  from {
    opacity: 0;
    transform: translateY(-20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* Scrollbar styles */
.messages-container::-webkit-scrollbar,
.history-panel::-webkit-scrollbar {
  width: 6px;
}

.messages-container::-webkit-scrollbar-track,
.history-panel::-webkit-scrollbar-track {
  background: var(--el-fill-color-lighter);
  border-radius: 3px;
}

.messages-container::-webkit-scrollbar-thumb,
.history-panel::-webkit-scrollbar-thumb {
  background: var(--el-border-color);
  border-radius: 3px;
  transition: background 0.3s;
}

.messages-container::-webkit-scrollbar-thumb:hover,
.history-panel::-webkit-scrollbar-thumb:hover {
  background: var(--el-border-color-darker);
}

/* Loading animation */
.is-loading {
  animation: rotating 2s linear infinite;
}

@keyframes rotating {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* Typing cursor animation for streaming */
.typing-cursor {
  display: inline-block;
  margin-left: 2px;
  animation: blink 1s step-end infinite;
  color: var(--el-color-primary);
  font-weight: bold;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Thinking section styles */
.thinking-section {
  margin-bottom: 16px;
  border-radius: 8px;
  border: 1px solid rgba(99, 102, 241, 0.2);
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.03) 0%, rgba(147, 197, 253, 0.02) 100%);
  overflow: hidden;
  transition: all 0.3s ease;
}

.thinking-section:hover {
  border-color: rgba(99, 102, 241, 0.3);
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.1);
}

.dark .thinking-section {
  border-color: rgba(99, 102, 241, 0.25);
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.08) 0%, rgba(147, 197, 253, 0.05) 100%);
}

.dark .thinking-section:hover {
  border-color: rgba(99, 102, 241, 0.35);
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.15);
}

.thinking-section-compare {
  margin-bottom: 12px;
}

.thinking-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: rgba(99, 102, 241, 0.05);
  border-bottom: 1px solid rgba(99, 102, 241, 0.1);
  cursor: pointer;
  user-select: none;
  transition: all 0.2s ease;
}

.thinking-header:hover {
  background: rgba(99, 102, 241, 0.08);
}

.dark .thinking-header {
  background: rgba(99, 102, 241, 0.12);
  border-bottom-color: rgba(99, 102, 241, 0.15);
}

.dark .thinking-header:hover {
  background: rgba(99, 102, 241, 0.15);
}

.thinking-label {
  font-size: 13px;
  font-weight: 500;
  color: rgba(99, 102, 241, 1);
  flex: 1;
}

.dark .thinking-label {
  color: rgba(147, 197, 253, 1);
}

.thinking-toggle {
  font-size: 12px;
  padding: 4px 8px;
  height: auto;
  color: rgba(99, 102, 241, 0.8);
}

.thinking-toggle:hover {
  color: rgba(99, 102, 241, 1);
  background: rgba(99, 102, 241, 0.1);
}

.dark .thinking-toggle {
  color: rgba(147, 197, 253, 0.8);
}

.dark .thinking-toggle:hover {
  color: rgba(147, 197, 253, 1);
  background: rgba(99, 102, 241, 0.15);
}

.thinking-content {
  padding: 14px;
  background: rgba(255, 255, 255, 0.5);
  animation: slideDown 0.3s ease;
  max-height: 400px;
  overflow-y: auto;
}

.dark .thinking-content {
  background: rgba(30, 30, 35, 0.5);
}

.thinking-content .markdown-body {
  font-size: 13px;
  line-height: 1.6;
  color: #606266;
}

.dark .thinking-content .markdown-body {
  color: #ccc;
}

/* Thinking content scrollbar */
.thinking-content::-webkit-scrollbar {
  width: 4px;
}

.thinking-content::-webkit-scrollbar-track {
  background: rgba(99, 102, 241, 0.05);
  border-radius: 2px;
}

.thinking-content::-webkit-scrollbar-thumb {
  background: rgba(99, 102, 241, 0.3);
  border-radius: 2px;
}

.thinking-content::-webkit-scrollbar-thumb:hover {
  background: rgba(99, 102, 241, 0.5);
}

/* Debug info styles */
.debug-info {
  margin-bottom: 12px;
  padding: 12px;
  background: #fff3cd;
  border: 1px solid #ffc107;
  border-radius: 6px;
  font-size: 12px;
}

.dark .debug-info {
  background: rgba(255, 193, 7, 0.1);
  border-color: rgba(255, 193, 7, 0.3);
}

.debug-section {
  margin-bottom: 8px;
}

.debug-section:last-child {
  margin-bottom: 0;
}

.debug-section strong {
  display: block;
  margin-bottom: 4px;
  color: #856404;
}

.dark .debug-section strong {
  color: #ffc107;
}

.debug-section pre {
  margin: 0;
  padding: 6px;
  background: rgba(0, 0, 0, 0.05);
  border-radius: 3px;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 11px;
  color: #333;
}

.dark .debug-section pre {
  background: rgba(0, 0, 0, 0.3);
  color: #ddd;
}

/* Responsive styles */
@media (max-width: 768px) {
  /* chat-content now uses flex layout, no need for specific height */

  .message-bubble {
    max-width: 85%;
  }

  .example-card {
    padding: 12px;
    font-size: 12px;
  }

  .empty-state-container h2 {
    font-size: 18px;
  }

  .empty-state-container .grid {
    display: none;
  }

  .input-hint {
    display: none;
  }

  /* Adjust split screen on mobile */
  .chat-split-screen {
    flex-direction: column;
  }

  .header-btn {
    padding: 0 12px;
    font-size: 13px;
  }

  .chat-input :deep(.el-textarea__inner),
  .chat-input-centered :deep(.el-textarea__inner) {
    padding-right: 60px;
  }
}

/* Utility classes */
.flex {
  display: flex;
}

.items-center {
  align-items: center;
}

.justify-between {
  justify-content: space-between;
}
</style>
