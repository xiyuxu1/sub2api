<!--
  我的账号（fork 新增）：普通用户自助导入并管理【自己的】上游账号。
  复用管理员的 CreateAccountModal / EditAccountModal / AccountTestModal，
  后端 /admin/accounts 已按 owner 收口，普通用户只看到/操作自己的号。
  新导入的账号默认不参与调度，需管理员审核后加入共享池。
  注：本页要套 <AppLayout>（对齐 Dashboard/Keys），否则没有侧边栏/顶栏。
  后续计划见 fork-docs/README.md §8.3（把账号管理大部分功能迁进来，做完整自助）。
-->
<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl">
      <div class="mb-6 flex items-center justify-end">
        <button
          class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700"
          @click="showCreate = true"
        >
          {{ t('myAccounts.import') }}
        </button>
      </div>

      <div class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('myAccounts.reviewNotice') }}
      </div>

      <div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead class="bg-gray-50 dark:bg-gray-900/40">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.name') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.platform') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.type') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.status') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.visibility') }}</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">{{ t('myAccounts.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-gray-700">
            <tr v-if="loading">
              <td colspan="6" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('common.loading') }}</td>
            </tr>
            <tr v-else-if="accounts.length === 0">
              <td colspan="6" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('myAccounts.empty') }}</td>
            </tr>
            <tr v-for="acc in accounts" :key="acc.id" class="hover:bg-gray-50 dark:hover:bg-gray-700/40">
              <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{{ acc.name }}</td>
              <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ acc.platform }}</td>
              <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ acc.type }}</td>
              <td class="px-4 py-3 text-sm">
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs" :class="statusClass(acc.status)">
                  {{ acc.status }}
                </span>
              </td>
              <td class="px-4 py-3 text-sm">
                <button
                  type="button"
                  :title="t('myAccounts.publicHint')"
                  :disabled="togglingId === acc.id"
                  class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium transition disabled:opacity-50"
                  :class="acc.is_public
                    ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'bg-gray-100 text-gray-500 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300'"
                  @click="togglePublic(acc)"
                >
                  {{ acc.is_public ? t('myAccounts.public') : t('myAccounts.private') }}
                </button>
              </td>
              <td class="px-4 py-3 text-right text-sm">
                <button class="mr-3 text-gray-500 hover:text-primary-600" @click="openEdit(acc)">{{ t('common.edit') }}</button>
                <button class="mr-3 text-gray-500 hover:text-primary-600" @click="openTest(acc)">{{ t('myAccounts.test') }}</button>
                <button class="text-gray-500 hover:text-red-600" @click="remove(acc)">{{ t('common.delete') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 复用管理员的富导入弹窗；普通用户不绑分组/代理，传空数组即可（后端也会强制 clamp）。 -->
      <CreateAccountModal :show="showCreate" :proxies="[]" :groups="[]" @close="showCreate = false" @created="onChanged" />
      <EditAccountModal :show="showEdit" :account="editing" :proxies="[]" :groups="[]" @close="showEdit = false" @updated="onChanged" />
      <AccountTestModal :show="showTest" :account="testing" @close="showTest = false" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import CreateAccountModal from '@/components/account/CreateAccountModal.vue'
import EditAccountModal from '@/components/account/EditAccountModal.vue'
import AccountTestModal from '@/components/account/AccountTestModal.vue'
import type { Account } from '@/types'

const { t } = useI18n()

const accounts = ref<Account[]>([])
const loading = ref(false)
const showCreate = ref(false)
const showEdit = ref(false)
const showTest = ref(false)
const editing = ref<Account | null>(null)
const testing = ref<Account | null>(null)
const togglingId = ref<number | null>(null)

async function reload() {
  loading.value = true
  try {
    const res = await adminAPI.accounts.list(1, 200)
    accounts.value = res.items ?? []
  } finally {
    loading.value = false
  }
}

// 翻转"公开/私有"。乐观更新，失败回滚。后端已按 owner 收口，只能改自己的号。
async function togglePublic(acc: Account) {
  if (togglingId.value !== null) return
  const next = !acc.is_public
  togglingId.value = acc.id
  acc.is_public = next
  try {
    await adminAPI.accounts.update(acc.id, { is_public: next })
  } catch (e) {
    acc.is_public = !next // 回滚
    window.alert(e instanceof Error ? e.message : String(e))
  } finally {
    togglingId.value = null
  }
}

function openEdit(acc: Account) {
  editing.value = acc
  showEdit.value = true
}

function openTest(acc: Account) {
  testing.value = acc
  showTest.value = true
}

async function remove(acc: Account) {
  if (!window.confirm(t('myAccounts.confirmDelete', { name: acc.name }))) return
  await adminAPI.accounts.delete(acc.id)
  await reload()
}

function onChanged() {
  showCreate.value = false
  showEdit.value = false
  reload()
}

function statusClass(status: string) {
  if (status === 'active') return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
  if (status === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  return 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
}

onMounted(reload)
</script>
