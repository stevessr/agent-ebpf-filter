import * as $protobuf from "protobufjs";
import Long = require("long");
/** Namespace pb. */
export namespace pb {

    /** Properties of a RegisterRequest. */
    interface IRegisterRequest {

        /** RegisterRequest pid */
        pid?: (number|null);
    }

    /** Represents a RegisterRequest. */
    class RegisterRequest implements IRegisterRequest {

        /**
         * Constructs a new RegisterRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRegisterRequest);

        /** RegisterRequest pid. */
        public pid: number;

        /**
         * Creates a new RegisterRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RegisterRequest instance
         */
        public static create(properties?: pb.IRegisterRequest): pb.RegisterRequest;

        /**
         * Encodes the specified RegisterRequest message. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @param message RegisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRegisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RegisterRequest message, length delimited. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @param message RegisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRegisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RegisterRequest;

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RegisterRequest;

        /**
         * Verifies a RegisterRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RegisterRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RegisterRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.RegisterRequest;

        /**
         * Creates a plain object from a RegisterRequest message. Also converts values to other types if specified.
         * @param message RegisterRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RegisterRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RegisterRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RegisterRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a RegisterResponse. */
    interface IRegisterResponse {

        /** RegisterResponse success */
        success?: (boolean|null);

        /** RegisterResponse message */
        message?: (string|null);
    }

    /** Represents a RegisterResponse. */
    class RegisterResponse implements IRegisterResponse {

        /**
         * Constructs a new RegisterResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRegisterResponse);

        /** RegisterResponse success. */
        public success: boolean;

        /** RegisterResponse message. */
        public message: string;

        /**
         * Creates a new RegisterResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RegisterResponse instance
         */
        public static create(properties?: pb.IRegisterResponse): pb.RegisterResponse;

        /**
         * Encodes the specified RegisterResponse message. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @param message RegisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRegisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RegisterResponse message, length delimited. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @param message RegisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRegisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RegisterResponse;

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RegisterResponse;

        /**
         * Verifies a RegisterResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RegisterResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RegisterResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.RegisterResponse;

        /**
         * Creates a plain object from a RegisterResponse message. Also converts values to other types if specified.
         * @param message RegisterResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RegisterResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RegisterResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RegisterResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an UnregisterRequest. */
    interface IUnregisterRequest {

        /** UnregisterRequest pid */
        pid?: (number|null);
    }

    /** Represents an UnregisterRequest. */
    class UnregisterRequest implements IUnregisterRequest {

        /**
         * Constructs a new UnregisterRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IUnregisterRequest);

        /** UnregisterRequest pid. */
        public pid: number;

        /**
         * Creates a new UnregisterRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns UnregisterRequest instance
         */
        public static create(properties?: pb.IUnregisterRequest): pb.UnregisterRequest;

        /**
         * Encodes the specified UnregisterRequest message. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @param message UnregisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IUnregisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified UnregisterRequest message, length delimited. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @param message UnregisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IUnregisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.UnregisterRequest;

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.UnregisterRequest;

        /**
         * Verifies an UnregisterRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an UnregisterRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns UnregisterRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.UnregisterRequest;

        /**
         * Creates a plain object from an UnregisterRequest message. Also converts values to other types if specified.
         * @param message UnregisterRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.UnregisterRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this UnregisterRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for UnregisterRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an UnregisterResponse. */
    interface IUnregisterResponse {

        /** UnregisterResponse success */
        success?: (boolean|null);

        /** UnregisterResponse message */
        message?: (string|null);
    }

    /** Represents an UnregisterResponse. */
    class UnregisterResponse implements IUnregisterResponse {

        /**
         * Constructs a new UnregisterResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IUnregisterResponse);

        /** UnregisterResponse success. */
        public success: boolean;

        /** UnregisterResponse message. */
        public message: string;

        /**
         * Creates a new UnregisterResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns UnregisterResponse instance
         */
        public static create(properties?: pb.IUnregisterResponse): pb.UnregisterResponse;

        /**
         * Encodes the specified UnregisterResponse message. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @param message UnregisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IUnregisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified UnregisterResponse message, length delimited. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @param message UnregisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IUnregisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.UnregisterResponse;

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.UnregisterResponse;

        /**
         * Verifies an UnregisterResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an UnregisterResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns UnregisterResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.UnregisterResponse;

        /**
         * Creates a plain object from an UnregisterResponse message. Also converts values to other types if specified.
         * @param message UnregisterResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.UnregisterResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this UnregisterResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for UnregisterResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an Event. */
    interface IEvent {

        /** Event pid */
        pid?: (number|null);

        /** Event ppid */
        ppid?: (number|null);

        /** Event uid */
        uid?: (number|null);

        /** Event type */
        type?: (string|null);

        /** Event tag */
        tag?: (string|null);

        /** Event comm */
        comm?: (string|null);

        /** Event path */
        path?: (string|null);
    }

    /** Represents an Event. */
    class Event implements IEvent {

        /**
         * Constructs a new Event.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IEvent);

        /** Event pid. */
        public pid: number;

        /** Event ppid. */
        public ppid: number;

        /** Event uid. */
        public uid: number;

        /** Event type. */
        public type: string;

        /** Event tag. */
        public tag: string;

        /** Event comm. */
        public comm: string;

        /** Event path. */
        public path: string;

        /**
         * Creates a new Event instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Event instance
         */
        public static create(properties?: pb.IEvent): pb.Event;

        /**
         * Encodes the specified Event message. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @param message Event message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Event message, length delimited. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @param message Event message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an Event message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.Event;

        /**
         * Decodes an Event message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.Event;

        /**
         * Verifies an Event message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an Event message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Event
         */
        public static fromObject(object: { [k: string]: any }): pb.Event;

        /**
         * Creates a plain object from an Event message. Also converts values to other types if specified.
         * @param message Event
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.Event, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Event to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Event
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a Process. */
    interface IProcess {

        /** Process pid */
        pid?: (number|null);

        /** Process ppid */
        ppid?: (number|null);

        /** Process name */
        name?: (string|null);

        /** Process cpu */
        cpu?: (number|null);

        /** Process mem */
        mem?: (number|null);

        /** Process user */
        user?: (string|null);

        /** Process gpuMem */
        gpuMem?: (number|null);

        /** Process gpuUtil */
        gpuUtil?: (number|null);

        /** Process gpuId */
        gpuId?: (number|null);
    }

    /** Represents a Process. */
    class Process implements IProcess {

        /**
         * Constructs a new Process.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IProcess);

        /** Process pid. */
        public pid: number;

        /** Process ppid. */
        public ppid: number;

        /** Process name. */
        public name: string;

        /** Process cpu. */
        public cpu: number;

        /** Process mem. */
        public mem: number;

        /** Process user. */
        public user: string;

        /** Process gpuMem. */
        public gpuMem: number;

        /** Process gpuUtil. */
        public gpuUtil: number;

        /** Process gpuId. */
        public gpuId: number;

        /**
         * Creates a new Process instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Process instance
         */
        public static create(properties?: pb.IProcess): pb.Process;

        /**
         * Encodes the specified Process message. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @param message Process message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IProcess, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Process message, length delimited. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @param message Process message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IProcess, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a Process message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.Process;

        /**
         * Decodes a Process message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.Process;

        /**
         * Verifies a Process message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a Process message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Process
         */
        public static fromObject(object: { [k: string]: any }): pb.Process;

        /**
         * Creates a plain object from a Process message. Also converts values to other types if specified.
         * @param message Process
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.Process, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Process to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Process
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a GPUStatus. */
    interface IGPUStatus {

        /** GPUStatus index */
        index?: (number|null);

        /** GPUStatus name */
        name?: (string|null);

        /** GPUStatus utilGpu */
        utilGpu?: (number|null);

        /** GPUStatus utilMem */
        utilMem?: (number|null);

        /** GPUStatus memTotal */
        memTotal?: (number|null);

        /** GPUStatus memUsed */
        memUsed?: (number|null);

        /** GPUStatus temp */
        temp?: (number|null);
    }

    /** Represents a GPUStatus. */
    class GPUStatus implements IGPUStatus {

        /**
         * Constructs a new GPUStatus.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IGPUStatus);

        /** GPUStatus index. */
        public index: number;

        /** GPUStatus name. */
        public name: string;

        /** GPUStatus utilGpu. */
        public utilGpu: number;

        /** GPUStatus utilMem. */
        public utilMem: number;

        /** GPUStatus memTotal. */
        public memTotal: number;

        /** GPUStatus memUsed. */
        public memUsed: number;

        /** GPUStatus temp. */
        public temp: number;

        /**
         * Creates a new GPUStatus instance using the specified properties.
         * @param [properties] Properties to set
         * @returns GPUStatus instance
         */
        public static create(properties?: pb.IGPUStatus): pb.GPUStatus;

        /**
         * Encodes the specified GPUStatus message. Does not implicitly {@link pb.GPUStatus.verify|verify} messages.
         * @param message GPUStatus message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IGPUStatus, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified GPUStatus message, length delimited. Does not implicitly {@link pb.GPUStatus.verify|verify} messages.
         * @param message GPUStatus message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IGPUStatus, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a GPUStatus message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns GPUStatus
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.GPUStatus;

        /**
         * Decodes a GPUStatus message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns GPUStatus
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.GPUStatus;

        /**
         * Verifies a GPUStatus message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a GPUStatus message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns GPUStatus
         */
        public static fromObject(object: { [k: string]: any }): pb.GPUStatus;

        /**
         * Creates a plain object from a GPUStatus message. Also converts values to other types if specified.
         * @param message GPUStatus
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.GPUStatus, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this GPUStatus to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for GPUStatus
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a SystemStats. */
    interface ISystemStats {

        /** SystemStats processes */
        processes?: (pb.IProcess[]|null);

        /** SystemStats gpus */
        gpus?: (pb.IGPUStatus[]|null);
    }

    /** Represents a SystemStats. */
    class SystemStats implements ISystemStats {

        /**
         * Constructs a new SystemStats.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ISystemStats);

        /** SystemStats processes. */
        public processes: pb.IProcess[];

        /** SystemStats gpus. */
        public gpus: pb.IGPUStatus[];

        /**
         * Creates a new SystemStats instance using the specified properties.
         * @param [properties] Properties to set
         * @returns SystemStats instance
         */
        public static create(properties?: pb.ISystemStats): pb.SystemStats;

        /**
         * Encodes the specified SystemStats message. Does not implicitly {@link pb.SystemStats.verify|verify} messages.
         * @param message SystemStats message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ISystemStats, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified SystemStats message, length delimited. Does not implicitly {@link pb.SystemStats.verify|verify} messages.
         * @param message SystemStats message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ISystemStats, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a SystemStats message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns SystemStats
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.SystemStats;

        /**
         * Decodes a SystemStats message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns SystemStats
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.SystemStats;

        /**
         * Verifies a SystemStats message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a SystemStats message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns SystemStats
         */
        public static fromObject(object: { [k: string]: any }): pb.SystemStats;

        /**
         * Creates a plain object from a SystemStats message. Also converts values to other types if specified.
         * @param message SystemStats
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.SystemStats, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this SystemStats to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for SystemStats
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a WrapperRequest. */
    interface IWrapperRequest {

        /** WrapperRequest pid */
        pid?: (number|null);

        /** WrapperRequest comm */
        comm?: (string|null);

        /** WrapperRequest args */
        args?: (string[]|null);

        /** WrapperRequest user */
        user?: (string|null);
    }

    /** Represents a WrapperRequest. */
    class WrapperRequest implements IWrapperRequest {

        /**
         * Constructs a new WrapperRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IWrapperRequest);

        /** WrapperRequest pid. */
        public pid: number;

        /** WrapperRequest comm. */
        public comm: string;

        /** WrapperRequest args. */
        public args: string[];

        /** WrapperRequest user. */
        public user: string;

        /**
         * Creates a new WrapperRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns WrapperRequest instance
         */
        public static create(properties?: pb.IWrapperRequest): pb.WrapperRequest;

        /**
         * Encodes the specified WrapperRequest message. Does not implicitly {@link pb.WrapperRequest.verify|verify} messages.
         * @param message WrapperRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IWrapperRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified WrapperRequest message, length delimited. Does not implicitly {@link pb.WrapperRequest.verify|verify} messages.
         * @param message WrapperRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IWrapperRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a WrapperRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns WrapperRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.WrapperRequest;

        /**
         * Decodes a WrapperRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns WrapperRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.WrapperRequest;

        /**
         * Verifies a WrapperRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a WrapperRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns WrapperRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.WrapperRequest;

        /**
         * Creates a plain object from a WrapperRequest message. Also converts values to other types if specified.
         * @param message WrapperRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.WrapperRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this WrapperRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for WrapperRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a WrapperResponse. */
    interface IWrapperResponse {

        /** WrapperResponse action */
        action?: (pb.WrapperResponse.Action|null);

        /** WrapperResponse message */
        message?: (string|null);

        /** WrapperResponse rewrittenArgs */
        rewrittenArgs?: (string[]|null);
    }

    /** Represents a WrapperResponse. */
    class WrapperResponse implements IWrapperResponse {

        /**
         * Constructs a new WrapperResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IWrapperResponse);

        /** WrapperResponse action. */
        public action: pb.WrapperResponse.Action;

        /** WrapperResponse message. */
        public message: string;

        /** WrapperResponse rewrittenArgs. */
        public rewrittenArgs: string[];

        /**
         * Creates a new WrapperResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns WrapperResponse instance
         */
        public static create(properties?: pb.IWrapperResponse): pb.WrapperResponse;

        /**
         * Encodes the specified WrapperResponse message. Does not implicitly {@link pb.WrapperResponse.verify|verify} messages.
         * @param message WrapperResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IWrapperResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified WrapperResponse message, length delimited. Does not implicitly {@link pb.WrapperResponse.verify|verify} messages.
         * @param message WrapperResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IWrapperResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a WrapperResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns WrapperResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.WrapperResponse;

        /**
         * Decodes a WrapperResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns WrapperResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.WrapperResponse;

        /**
         * Verifies a WrapperResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a WrapperResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns WrapperResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.WrapperResponse;

        /**
         * Creates a plain object from a WrapperResponse message. Also converts values to other types if specified.
         * @param message WrapperResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.WrapperResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this WrapperResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for WrapperResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    namespace WrapperResponse {

        /** Action enum. */
        enum Action {
            ALLOW = 0,
            BLOCK = 1,
            REWRITE = 2,
            ALERT = 3
        }
    }

    /** Properties of a ProcessList. */
    interface IProcessList {

        /** ProcessList processes */
        processes?: (pb.IProcess[]|null);
    }

    /** Represents a ProcessList. */
    class ProcessList implements IProcessList {

        /**
         * Constructs a new ProcessList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IProcessList);

        /** ProcessList processes. */
        public processes: pb.IProcess[];

        /**
         * Creates a new ProcessList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ProcessList instance
         */
        public static create(properties?: pb.IProcessList): pb.ProcessList;

        /**
         * Encodes the specified ProcessList message. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @param message ProcessList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IProcessList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ProcessList message, length delimited. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @param message ProcessList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IProcessList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ProcessList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ProcessList;

        /**
         * Decodes a ProcessList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ProcessList;

        /**
         * Verifies a ProcessList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ProcessList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ProcessList
         */
        public static fromObject(object: { [k: string]: any }): pb.ProcessList;

        /**
         * Creates a plain object from a ProcessList message. Also converts values to other types if specified.
         * @param message ProcessList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ProcessList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ProcessList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ProcessList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }
}
