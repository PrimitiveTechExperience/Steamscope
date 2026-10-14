import { LOCAL_RECENT_KEY, loadLocalRecentSearches, saveLocalRecentSearches } from './local-recent-searches';

describe('recent searches kept in the browser', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it('reads back what was saved', () => {
    saveLocalRecentSearches([{ search: 'portal', include_bundles: true }, { genres: ['RPG'], min_discount: 50 }]);
    expect(loadLocalRecentSearches(5)).toEqual([{ search: 'portal', include_bundles: true }, { genres: ['RPG'], min_discount: 50 }]);
  });

  it('is empty when nothing was saved', () => {
    expect(loadLocalRecentSearches(5)).toEqual([]);
  });

  it('returns no more than asked for', () => {
    saveLocalRecentSearches(Array.from({ length: 9 }, (_, i) => ({ search: `s${i}` })));
    expect(loadLocalRecentSearches(5)).toHaveLength(5);
  });

  it.each([
    ['not JSON', '{oops'],
    ['not a list', '{"search":"x"}'],
    ['a string', '"portal"'],
    ['null', 'null'],
  ])('ignores stored data that is %s', (_name, raw) => {
    localStorage.setItem(LOCAL_RECENT_KEY, raw);
    expect(loadLocalRecentSearches(5)).toEqual([]);
  });

  it('drops entries that are not searches, and fields of the wrong kind', () => {
    localStorage.setItem(
      LOCAL_RECENT_KEY,
      JSON.stringify([
        null,
        42,
        'text',
        [],
        {},
        { search: '   ' },
        { bogus: 'field' },
        { search: 'kept', genres: ['RPG', 7, '', null], min_price: -3, max_price: 'cheap', min_discount: 500, include_bundles: 'yes' },
        { min_discount: 25, max_price: 20 },
      ])
    );
    expect(loadLocalRecentSearches(10)).toEqual([{ search: 'kept', genres: ['RPG'] }, { min_discount: 25, max_price: 20 }]);
  });

  it('caps the length of what it reads back', () => {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify([{ search: 'x'.repeat(1000), tags: Array.from({ length: 100 }, (_, i) => `t${i}`) }]));
    const [s] = loadLocalRecentSearches(5);
    expect(s.search!.length).toBe(100);
    expect(s.tags!.length).toBe(30);
  });

  it('copes with storage being unavailable', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    expect(loadLocalRecentSearches(5)).toEqual([]);
    expect(() => saveLocalRecentSearches([{ search: 'x' }])).not.toThrow();
  });
});
