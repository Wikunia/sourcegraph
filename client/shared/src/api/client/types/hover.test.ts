import { MarkupKind } from '@sourcegraph/extension-api-classes'
import { Range } from '@sourcegraph/extension-api-types'
import { fromHoverMerged } from './hover'

const FIXTURE_RANGE: Range = { start: { line: 1, character: 2 }, end: { line: 3, character: 4 } }

describe('HoverMerged', () => {
    describe('from', () => {
        test('0 hovers', () => expect(fromHoverMerged([])).toBeNull())
        test('empty hovers', () => expect(fromHoverMerged([null, undefined])).toBeNull())
        test('empty string hovers', () => expect(fromHoverMerged([{ contents: { value: '' } }])).toBeNull())
        test('1 MarkupContent', () =>
            expect(fromHoverMerged([{ contents: { kind: MarkupKind.Markdown, value: 'x' } }])).toEqual({
                contents: [{ kind: MarkupKind.Markdown, value: 'x' }],
                alerts: [],
                aggregatedBadges: [],
            }))
        test('2 MarkupContents', () =>
            expect(
                fromHoverMerged([
                    { contents: { kind: MarkupKind.Markdown, value: 'x' }, range: FIXTURE_RANGE },
                    { contents: { kind: MarkupKind.Markdown, value: 'y' } },
                ])
            ).toEqual({
                contents: [
                    { kind: MarkupKind.Markdown, value: 'x' },
                    { kind: MarkupKind.Markdown, value: 'y' },
                ],
                range: FIXTURE_RANGE,
                alerts: [],
                aggregatedBadges: [],
            }))
        test('1 Alert', () =>
            expect(
                fromHoverMerged([
                    {
                        contents: { kind: MarkupKind.Markdown, value: 'x' },
                        alerts: [{ summary: { kind: MarkupKind.PlainText, value: 'x' } }],
                    },
                ])
            ).toEqual({
                contents: [{ kind: MarkupKind.Markdown, value: 'x' }],
                alerts: [{ summary: { kind: MarkupKind.PlainText, value: 'x' } }],
                aggregatedBadges: [],
            }))
        test('2 Alerts', () =>
            expect(
                fromHoverMerged([
                    {
                        contents: { kind: MarkupKind.Markdown, value: 'x' },
                        alerts: [
                            { summary: { kind: MarkupKind.PlainText, value: 'x' } },
                            { summary: { kind: MarkupKind.PlainText, value: 'y' } },
                        ],
                    },
                ])
            ).toEqual({
                contents: [{ kind: MarkupKind.Markdown, value: 'x' }],
                alerts: [
                    { summary: { kind: MarkupKind.PlainText, value: 'x' } },
                    { summary: { kind: MarkupKind.PlainText, value: 'y' } },
                ],
                aggregatedBadges: [],
            }))

        test('Aggregated Badges', () =>
            expect(
                fromHoverMerged([
                    {
                        contents: { kind: MarkupKind.Markdown, value: 'x' },
                        alerts: [],
                        aggregableBadges: [{ text: 't01' }, { text: 't03' }],
                    },
                    {
                        contents: { kind: MarkupKind.Markdown, value: 'y' },
                        alerts: [],
                        aggregableBadges: [{ text: 't02' }],
                    },
                ])
            ).toEqual({
                contents: [
                    { kind: MarkupKind.Markdown, value: 'x' },
                    { kind: MarkupKind.Markdown, value: 'y' },
                ],
                alerts: [],
                aggregatedBadges: [{ text: 't01' }, { text: 't02' }, { text: 't03' }],
            }))
    })
})
