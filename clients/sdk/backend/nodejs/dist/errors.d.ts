export declare class ApiError extends Error {
    status: number;
    body: any;
    constructor(status: number, body: any);
}
