<template>
  <div class="no-access">
    <div class="card">
      <h1 class="title">No access yet</h1>
      <p class="lead">
        Your account is signed in, but it has not been given access to anything on this platform.
        Ask the contact below to grant it, then check again.
      </p>

      <dl class="account">
        <dt>Name</dt>
        <dd>{{ name || '-' }}</dd>
        <dt>Email</dt>
        <dd>{{ email || '-' }}</dd>
        <dt>ID</dt>
        <dd>
          <span class="mono">{{ id || '-' }}</span>
          <el-button v-if="id" link type="primary" @click="copyId">Copy ID</el-button>
        </dd>
        <dt>Roles</dt>
        <dd>{{ rolesText || '-' }}</dd>
      </dl>

      <!-- Interpolated, so it is shown as text whatever it contains. -->
      <p v-if="contact" class="contact">{{ contact }}</p>

      <p v-if="stillNoAccess" class="notice" role="status">
        Still no access. Ask your contact to grant it, then check again.
      </p>

      <div class="actions">
        <el-button type="primary" :loading="checking" @click="checkAgain">Check again</el-button>
        <el-button :loading="signingOut" @click="signOut">Sign out</el-button>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useNoAccess } from './useNoAccess'

const {
  name,
  email,
  id,
  rolesText,
  contact,
  checking,
  signingOut,
  stillNoAccess,
  loadContact,
  copyId,
  checkAgain,
  signOut,
} = useNoAccess(useRouter())

onMounted(loadContact)
</script>

<style scoped lang="scss">
.no-access {
  min-height: 100svh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: var(--el-bg-color-page);
}

.card {
  width: 100%;
  max-width: 560px;
  padding: 40px;
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: var(--el-box-shadow-light);
}

.title {
  margin: 0 0 12px;
  font-size: 24px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.lead {
  margin: 0 0 24px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
}

.account {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 8px 16px;
  margin: 0 0 24px;
  padding: 16px;
  border-radius: 8px;
  background: var(--el-fill-color-light);

  dt {
    color: var(--el-text-color-secondary);
  }

  dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
    color: var(--el-text-color-primary);
  }
}

.mono {
  font-family: var(--el-font-family-mono, ui-monospace, monospace);
}

.contact {
  margin: 0 0 24px;
  line-height: 1.6;
  white-space: pre-line;
  color: var(--el-text-color-primary);
}

.notice {
  margin: 0 0 24px;
  color: var(--el-color-warning);
}

.actions {
  display: flex;
  gap: 12px;
}
</style>
