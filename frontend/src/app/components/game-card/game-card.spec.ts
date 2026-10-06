import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { GameCardComponent } from './game-card';
import { makeGame, textOf } from '../../../testing/factories';

function render(over = {}) {
  TestBed.configureTestingModule({ imports: [GameCardComponent], providers: [provideRouter([])] });
  const fixture = TestBed.createComponent(GameCardComponent);
  fixture.componentRef.setInput('game', makeGame(over));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('GameCardComponent', () => {
  it('shows the name, current price, struck-through original price and discount badge', () => {
    const el = render();
    const text = textOf(el);
    expect(text).toContain('Counter-Strike 2');
    expect(text).toContain('$9.99');
    expect(text).toContain('$19.99');
    expect(text).toContain('-50%');
    expect(el.querySelector('.line-through')?.textContent).toContain('$19.99');
  });

  it('links to the game page', () => {
    expect(render().querySelector('a')?.getAttribute('href')).toBe('/games/730');
  });

  it('hides the discount badge and original price when not on sale', () => {
    const el = render({ discount_percentage: 0, original_price: 9.99 });
    expect(textOf(el)).not.toContain('-0%');
    expect(el.querySelector('.line-through')).toBeNull();
  });

  it('still shows the discount badge when the stored percentage is missing but the price is below the regular price', () => {
    const el = render({ price: 14.99, original_price: 59.99, discount_percentage: 0 });
    expect(textOf(el.querySelector('.edge-btn'))).toBe('-75%');
    expect(el.querySelector('.line-through')?.textContent).toContain('$59.99');
  });

  it('shows "Free" for a free game', () => {
    expect(textOf(render({ price: 0, original_price: 0, discount_percentage: 0 }))).toContain('Free');
  });

  it('strips HTML tags from the description preview', () => {
    const el = render({ description: '<h2>About This Game</h2><p class="bb_paragraph">A <b>tactical</b> shooter.</p>' });
    const preview = el.querySelector('p');
    expect(textOf(preview)).toBe('About This Game A tactical shooter.');
    expect(preview?.innerHTML).not.toContain('<');
  });

  it('uses the header image with the game name as alt text', () => {
    const img = render().querySelector('img');
    expect(img?.getAttribute('src')).toBe('https://cdn.example/730/header.jpg');
    expect(img?.getAttribute('alt')).toBe('Counter-Strike 2');
  });
});
