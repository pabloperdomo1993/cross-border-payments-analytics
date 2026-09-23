import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

import '@testing-library/jest-dom/vitest'

// RTL's automatic cleanup only self-registers when it detects Jest/Vitest
// globals; since this project imports `describe`/`it`/`afterEach` explicitly
// rather than enabling `test.globals`, it's wired up here instead.
afterEach(() => {
  cleanup()
})
