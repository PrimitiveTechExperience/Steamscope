import { Component, computed, input, output, signal } from '@angular/core';

/**
 * A search-as-you-type filter category: typing shows up to 5 matching
 * options, clicking one adds it as a removable chip. Used for genres, tags,
 * developers, publishers, and languages on the search page, where listing
 * every possible value as a checkbox would be unusable.
 */
@Component({
  selector: 'app-filter-autocomplete',
  templateUrl: './filter-autocomplete.html',
})
export class FilterAutocompleteComponent {
  label = input.required<string>();
  options = input.required<string[]>();
  selected = input.required<string[]>();
  toggle = output<string>();

  protected query = signal('');

  protected suggestions = computed(() => {
    const q = this.query().trim().toLowerCase();
    if (!q) return [];
    const selectedSet = new Set(this.selected());
    return this.options()
      .filter((option) => !selectedSet.has(option) && option.toLowerCase().includes(q))
      .slice(0, 5);
  });

  protected onInput(event: Event) {
    this.query.set((event.target as HTMLInputElement).value);
  }

  protected select(value: string) {
    this.toggle.emit(value);
    this.query.set('');
  }
}
