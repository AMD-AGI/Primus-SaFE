<template>
  <div class="quick-start-section">
    <div v-if="showHeader" class="quick-start-header">
      <span class="emoji-icon">{{ config.emoji }}</span>
      <span class="quick-title">{{ config.title }}</span>
    </div>
    <div class="quick-cards">
      <div
        v-for="(card, index) in config.cards"
        :key="index"
        class="quick-card"
        @click="handleCardClick(card)"
      >
        <el-icon v-if="card.icon" class="card-icon">
          <component :is="iconComponents[card.icon]" />
        </el-icon>
        <span v-else class="card-bullet">▸</span>
        <span class="card-text">{{ card.text }}</span>
        <span v-if="card.type === 'guided_workflow'" class="wizard-badge">Guided</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { QuestionFilled, Folder, Connection, Box } from '@element-plus/icons-vue'
import type { Component } from 'vue'
import type { QuickStartConfig, QuickStartCard } from '../constants/quickStartData'

interface Props {
  config: QuickStartConfig
  showHeader?: boolean
}

interface Emits {
  (e: 'cardClick', value: string): void
  (e: 'wizardClick', workflowId: string): void
}

withDefaults(defineProps<Props>(), {
  showHeader: true,
})

const emit = defineEmits<Emits>()

const iconComponents: Record<string, Component> = {
  QuestionFilled,
  Folder,
  Connection,
  Box,
}

const handleCardClick = (card: QuickStartCard) => {
  if (card.type === 'guided_workflow' && card.workflowId) {
    emit('wizardClick', card.workflowId)
  } else {
    emit('cardClick', card.value)
  }
}
</script>

<style lang="scss">
.quick-start-section {
  text-align: left;
  margin-bottom: 24px;

  .quick-start-header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 24px;
    padding-left: 4px;

    .emoji-icon {
      font-size: 22px;
    }

    .quick-title {
      font-size: 17px;
      font-weight: 700;
      color: #e2e8f0;
    }
  }

  .quick-cards {
    display: grid;
    gap: 14px;
  }

  .quick-card {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 16px 20px;
    background: rgba(241, 245, 249, 0.9);
    border: 1px solid rgba(203, 213, 225, 0.6);
    border-radius: 12px;
    cursor: pointer;
    transition: all 0.3s ease;
    position: relative;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

    .card-bullet {
      font-size: 15px;
      color: #64748b;
      font-weight: 600;
      flex-shrink: 0;
      transition: all 0.3s;
    }

    .card-icon {
      font-size: 16px;
      color: #64748b;
      flex-shrink: 0;
      transition: all 0.3s;
    }

    .card-text {
      font-size: 14px;
      color: #475569;
      line-height: 1.6;
      font-weight: 400;
      flex: 1;
      transition: all 0.3s;
    }

    &:hover {
      background: rgba(255, 255, 255, 0.95);
      border-color: rgba(20, 184, 166, 0.4);
      box-shadow: 0 8px 20px rgba(0, 0, 0, 0.1);
      transform: translateY(-3px) scale(1.02);

      .card-bullet {
        color: #14b8a6;
      }

      .card-icon {
        color: #14b8a6;
      }

      .card-text {
        color: #1e293b;
      }
    }

    .wizard-badge {
      flex-shrink: 0;
      font-size: 11px;
      font-weight: 600;
      color: var(--safe-primary);
      background: color-mix(in oklab, var(--safe-primary) 10%, transparent 90%);
      padding: 2px 8px;
      border-radius: 10px;
    }
  }
}

// ========== Light Mode ==========
:root:not(.dark) {
  .quick-start-section {
    .quick-start-header {
      .quick-title {
        color: #0f172a;
      }
    }

    .quick-card {
      background: rgba(241, 245, 249, 0.9);
      border: 1px solid rgba(203, 213, 225, 0.6);
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

      .card-bullet {
        color: #64748b;
      }

      .card-icon {
        color: #64748b;
      }

      .card-text {
        color: #475569;
      }

      &:hover {
        background: rgba(255, 255, 255, 0.95);
        border-color: rgba(20, 184, 166, 0.4);
        box-shadow: 0 8px 20px rgba(0, 0, 0, 0.1);
        transform: translateY(-3px) scale(1.02);

        .card-bullet {
          color: #14b8a6;
        }

        .card-icon {
          color: #14b8a6;
        }

        .card-text {
          color: #1e293b;
        }
      }
    }
  }
}

// ========== Dark Mode ==========
.dark {
  .quick-start-section {
    .quick-start-header {
      .quick-title {
        color: #e2e8f0;
      }
    }

    .quick-card {
      background: rgba(30, 41, 59, 0.4);
      border: 1px solid rgba(255, 255, 255, 0.12);
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);

      .card-icon {
        color: #94a3b8;
      }

      .card-text {
        color: #cbd5e1;
      }

      &:hover {
        background: rgba(30, 41, 59, 0.6);
        border-color: rgba(20, 184, 166, 0.3);
        box-shadow: 0 8px 20px rgba(0, 0, 0, 0.3);
        transform: translateY(-3px) scale(1.02);

        .card-icon {
          color: #14b8a6;
        }

        .card-text {
          color: #e2e8f0;
        }
      }

    }
  }
}
</style>
