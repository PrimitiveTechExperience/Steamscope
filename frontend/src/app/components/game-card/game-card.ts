import { Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Game } from '../../models/game';
import { CurrencyPipe } from '@angular/common';

@Component({
  selector: 'app-game-card',
  imports: [RouterLink, CurrencyPipe],
  templateUrl: './game-card.html',
  styleUrl: './game-card.css',
})
export class GameCardComponent {
  game = input.required<Game>();
}
