import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'

vi.mock('../context/SettingsContext', () => ({
  useSettings: () => ({ resolvedTheme: 'dark' }),
}))

import CodeBlock from './CodeBlock'

describe('CodeBlock', () => {
  it('highlights a registered language', () => {
    const { container } = render(
      <CodeBlock language="go" code={'func main() {}'} />,
    )
    expect(container.querySelector('.token')).not.toBeNull()
  })

  it('renders an unknown language as plain code without crashing', () => {
    const code = '++++[>++<-]>.'
    const { container } = render(<CodeBlock language="brainfuck" code={code} />)
    expect(container.querySelector('.token')).toBeNull()
    expect(container.textContent).toContain(code)
    expect(container.textContent).toContain('brainfuck')
  })
})
