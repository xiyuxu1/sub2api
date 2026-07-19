import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: '<div class="select-stub">{{ options.map(option => option.label).join("|") }}</div>'
}

describe('AccountTableFilters owner filter', () => {
  it('keeps all and system/admin options before individual users', async () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: { owner: '' },
        showOwnerFilter: true,
        users: [
          { id: 42, username: 'alice', email: 'alice@example.com' },
          { id: 43, username: 'bob@example.com', email: 'bob@example.com' },
          { id: 44, username: 'former', email: 'former@example.com', deleted_at: '2026-07-19T00:00:00Z' }
        ] as any
      },
      global: {
        stubs: { Select: SelectStub, SearchInput: true }
      }
    })

    const ownerSelect = wrapper.findAllComponents(SelectStub).at(-1)
    expect(ownerSelect?.props('options')).toEqual([
      { value: '', label: 'admin.accounts.allOwners' },
      { value: 'unassigned', label: 'admin.accounts.systemOwnedAccounts' },
      { value: '42', label: 'alice · alice@example.com' },
      { value: '43', label: 'bob@example.com' },
      { value: '44', label: 'admin.accounts.owner.deleted' }
    ])

    ownerSelect?.vm.$emit('update:modelValue', '42')
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:filters')?.at(-1)).toEqual([{ owner: '42' }])
  })
})
