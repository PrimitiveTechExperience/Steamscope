import { Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Game } from '../../models/game';
import { CurrencyPipe } from '@angular/common';
import { onHeaderImageError } from '../../utils/steam-image';
import { StripHtmlPipe } from '../../pipes/strip-html';

@Component({
  selector: 'app-game-card',
  imports: [RouterLink, CurrencyPipe, StripHtmlPipe],
  templateUrl: './game-card.html',
  styleUrl: './game-card.css',
})
export class GameCardComponent {
  game = input.required<Game>();
  onImageError = onHeaderImageError;
}
