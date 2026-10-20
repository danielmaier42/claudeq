import {cleanup, fireEvent, render, screen} from '@testing-library/react'
import {afterEach, describe, expect, it, vi} from 'vitest'
import {MarkdownBody} from './MarkdownBody'

const doc = `# Report

| Item | State |
| --- | --- |
| [#1 Card](https://dev.azure.com/x/1) | open |

- [x] done
- [ ] open

See [the Excel](</Users/dm/file.xlsx>) and ![chart](https://example.com/c.png).

<script>window.injected = true</script><b id="raw">raw</b>
`

describe('MarkdownBody', () => {
  afterEach(() => {
    cleanup()
    delete window.cqOpenExternal
  })

  it('renders headings, tables and task lists', () => {
    render(<MarkdownBody text={doc} />)
    expect(screen.getByRole('heading', {level: 1, name: 'Report'})).toBeTruthy()
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.getByRole('cell', {name: 'open'})).toBeTruthy()
    expect(screen.getAllByRole('checkbox')).toHaveLength(2)
  })

  it('sends web links to the browser and leaves other targets inert', () => {
    const open = vi.fn()
    window.cqOpenExternal = open
    render(<MarkdownBody text={doc} />)
    fireEvent.click(screen.getByRole('link', {name: '#1 Card'}))
    expect(open).toHaveBeenCalledWith('https://dev.azure.com/x/1')
    expect(screen.queryByRole('link', {name: 'the Excel'})).toBeNull()
    expect(screen.getByText('the Excel').getAttribute('title')).toBe('/Users/dm/file.xlsx')
  })

  it('loads no images and ignores raw HTML', () => {
    const {container} = render(<MarkdownBody text={doc} />)
    expect(container.querySelector('img')).toBeNull()
    expect(screen.getByText('[chart]')).toBeTruthy()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('#raw')).toBeNull()
  })
})
