import { Component, input } from '@angular/core';
import { CurrencyPipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { BundleArtComponent } from '../bundle-art/bundle-art';
import { Bundle } from '../../models/bundle';

@Component({
  selector: 'app-bundle-card',
  imports: [CurrencyPipe, RouterLink, BundleArtComponent],
  templateUrl: './bundle-card.html',
})
export class BundleCardComponent {
  bundle = input.required<Bundle>();
}
