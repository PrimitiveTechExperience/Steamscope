import { TestBed } from '@angular/core/testing';

import { Games } from './games';

describe('Games', () => {
  let service: Games;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(Games);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
