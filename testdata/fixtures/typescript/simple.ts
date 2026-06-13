import { EventEmitter } from 'events';

export interface Repository {
    findById(id: string): string;
}

export class UserService extends EventEmitter {
    constructor(private name: string) {
        super();
    }

    getName(): string {
        return this.name;
    }
}

export function createService(name: string): UserService {
    return new UserService(name);
}
