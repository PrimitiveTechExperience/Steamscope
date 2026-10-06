import { Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Game } from '../../models/game';
import { CurrencyPipe } from '@angular/common';
import { onHeaderImageError } from '../../utils/steam-image';
import { StripHtmlPipe } from '../../pipes/strip-html';
import { discountOf } from '../../utils/discount';

@Component({
  selector: 'app-game-card',
  imports: [RouterLink, CurrencyPipe, StripHtmlPipe],
  templateUrl: './game-card.html',
  styleUrl: './game-card.css',
})
export class GameCardComponent {
  game = input.required<Game>();
  protected discount = computed(() => discountOf(this.game()));
  onImageError = onHeaderImageError;
}
