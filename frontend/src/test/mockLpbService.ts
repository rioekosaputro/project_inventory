import { vi } from 'vitest'
import type * as Real from '@/services/lpbService'
import { pageOf } from '@/test/mockMasterService'

/**
 * Pengganti lpbService untuk test:
 *   vi.mock('@/services/lpbService', () => import('@/test/mockLpbService'))
 */
export const lpbService = {
  list: vi.fn<typeof Real.lpbService.list>(),
  approvals: vi.fn<typeof Real.lpbService.approvals>(),
  get: vi.fn<typeof Real.lpbService.get>(),
  create: vi.fn<typeof Real.lpbService.create>(),
  update: vi.fn<typeof Real.lpbService.update>(),
  addSerial: vi.fn<typeof Real.lpbService.addSerial>(),
  removeSerial: vi.fn<typeof Real.lpbService.removeSerial>(),
  submit: vi.fn<typeof Real.lpbService.submit>(),
  cancel: vi.fn<typeof Real.lpbService.cancel>(),
  revise: vi.fn<typeof Real.lpbService.revise>(),
  approve: vi.fn<typeof Real.lpbService.approve>(),
  reject: vi.fn<typeof Real.lpbService.reject>(),
}

export function resetLpbMocks(): void {
  for (const fn of Object.values(lpbService)) fn.mockReset()
  lpbService.list.mockResolvedValue(pageOf([]) as never)
  lpbService.approvals.mockResolvedValue(pageOf([]) as never)
}
