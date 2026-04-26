import * as $protobuf from "protobufjs";
import Long = require("long");
/** Namespace pb. */
export namespace pb {

    /** EventType enum. */
    enum EventType {
        EXECVE = 0,
        OPENAT = 1,
        NETWORK_CONNECT = 2,
        MKDIR = 3,
        UNLINK = 4,
        IOCTL = 5,
        NETWORK_BIND = 6,
        NETWORK_SENDTO = 7,
        NETWORK_RECVFROM = 8,
        READ = 9,
        WRITE = 10,
        OPEN = 11,
        CHMOD = 12,
        CHOWN = 13,
        RENAME = 14,
        LINK = 15,
        SYMLINK = 16,
        MKNOD = 17,
        CLONE = 18,
        EXIT = 19,
        SOCKET = 20,
        ACCEPT = 21,
        ACCEPT4 = 22,
        WRAPPER_INTERCEPT = 23,
        NATIVE_HOOK = 24
    }

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

        /** Event netDirection */
        netDirection?: (string|null);

        /** Event netEndpoint */
        netEndpoint?: (string|null);

        /** Event netBytes */
        netBytes?: (number|null);

        /** Event netFamily */
        netFamily?: (string|null);

        /** Event retval */
        retval?: (number|Long|null);

        /** Event extraInfo */
        extraInfo?: (string|null);

        /** Event extraPath */
        extraPath?: (string|null);

        /** Event bytes */
        bytes?: (number|Long|null);

        /** Event mode */
        mode?: (string|null);

        /** Event domain */
        domain?: (string|null);

        /** Event sockType */
        sockType?: (string|null);

        /** Event protocol */
        protocol?: (number|null);

        /** Event uidArg */
        uidArg?: (number|null);

        /** Event gidArg */
        gidArg?: (number|null);

        /** Event eventType */
        eventType?: (pb.EventType|null);
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

        /** Event netDirection. */
        public netDirection: string;

        /** Event netEndpoint. */
        public netEndpoint: string;

        /** Event netBytes. */
        public netBytes: number;

        /** Event netFamily. */
        public netFamily: string;

        /** Event retval. */
        public retval: (number|Long);

        /** Event extraInfo. */
        public extraInfo: string;

        /** Event extraPath. */
        public extraPath: string;

        /** Event bytes. */
        public bytes: (number|Long);

        /** Event mode. */
        public mode: string;

        /** Event domain. */
        public domain: string;

        /** Event sockType. */
        public sockType: string;

        /** Event protocol. */
        public protocol: number;

        /** Event uidArg. */
        public uidArg: number;

        /** Event gidArg. */
        public gidArg: number;

        /** Event eventType. */
        public eventType: pb.EventType;

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

    /** Properties of an EventBatch. */
    interface IEventBatch {

        /** EventBatch events */
        events?: (pb.IEvent[]|null);
    }

    /** Represents an EventBatch. */
    class EventBatch implements IEventBatch {

        /**
         * Constructs a new EventBatch.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IEventBatch);

        /** EventBatch events. */
        public events: pb.IEvent[];

        /**
         * Creates a new EventBatch instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EventBatch instance
         */
        public static create(properties?: pb.IEventBatch): pb.EventBatch;

        /**
         * Encodes the specified EventBatch message. Does not implicitly {@link pb.EventBatch.verify|verify} messages.
         * @param message EventBatch message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IEventBatch, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EventBatch message, length delimited. Does not implicitly {@link pb.EventBatch.verify|verify} messages.
         * @param message EventBatch message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IEventBatch, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EventBatch message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EventBatch
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.EventBatch;

        /**
         * Decodes an EventBatch message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EventBatch
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.EventBatch;

        /**
         * Verifies an EventBatch message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EventBatch message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EventBatch
         */
        public static fromObject(object: { [k: string]: any }): pb.EventBatch;

        /**
         * Creates a plain object from an EventBatch message. Also converts values to other types if specified.
         * @param message EventBatch
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.EventBatch, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EventBatch to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EventBatch
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

        /** Process cmdline */
        cmdline?: (string|null);

        /** Process createTime */
        createTime?: (number|Long|null);

        /** Process minorFaults */
        minorFaults?: (number|Long|null);

        /** Process majorFaults */
        majorFaults?: (number|Long|null);
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

        /** Process cmdline. */
        public cmdline: string;

        /** Process createTime. */
        public createTime: (number|Long);

        /** Process minorFaults. */
        public minorFaults: (number|Long);

        /** Process majorFaults. */
        public majorFaults: (number|Long);

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

    /** Properties of a CPUInfo. */
    interface ICPUInfo {

        /** CPUInfo total */
        total?: (number|null);

        /** CPUInfo cores */
        cores?: (number[]|null);

        /** CPUInfo coreDetails */
        coreDetails?: (pb.CPUInfo.ICore[]|null);
    }

    /** Represents a CPUInfo. */
    class CPUInfo implements ICPUInfo {

        /**
         * Constructs a new CPUInfo.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ICPUInfo);

        /** CPUInfo total. */
        public total: number;

        /** CPUInfo cores. */
        public cores: number[];

        /** CPUInfo coreDetails. */
        public coreDetails: pb.CPUInfo.ICore[];

        /**
         * Creates a new CPUInfo instance using the specified properties.
         * @param [properties] Properties to set
         * @returns CPUInfo instance
         */
        public static create(properties?: pb.ICPUInfo): pb.CPUInfo;

        /**
         * Encodes the specified CPUInfo message. Does not implicitly {@link pb.CPUInfo.verify|verify} messages.
         * @param message CPUInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ICPUInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified CPUInfo message, length delimited. Does not implicitly {@link pb.CPUInfo.verify|verify} messages.
         * @param message CPUInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ICPUInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a CPUInfo message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns CPUInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.CPUInfo;

        /**
         * Decodes a CPUInfo message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns CPUInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.CPUInfo;

        /**
         * Verifies a CPUInfo message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a CPUInfo message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns CPUInfo
         */
        public static fromObject(object: { [k: string]: any }): pb.CPUInfo;

        /**
         * Creates a plain object from a CPUInfo message. Also converts values to other types if specified.
         * @param message CPUInfo
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.CPUInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this CPUInfo to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for CPUInfo
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    namespace CPUInfo {

        /** Properties of a Core. */
        interface ICore {

            /** Core index */
            index?: (number|null);

            /** Core usage */
            usage?: (number|null);

            /** Core type */
            type?: (pb.CPUInfo.Core.Type|null);
        }

        /** Represents a Core. */
        class Core implements ICore {

            /**
             * Constructs a new Core.
             * @param [properties] Properties to set
             */
            constructor(properties?: pb.CPUInfo.ICore);

            /** Core index. */
            public index: number;

            /** Core usage. */
            public usage: number;

            /** Core type. */
            public type: pb.CPUInfo.Core.Type;

            /**
             * Creates a new Core instance using the specified properties.
             * @param [properties] Properties to set
             * @returns Core instance
             */
            public static create(properties?: pb.CPUInfo.ICore): pb.CPUInfo.Core;

            /**
             * Encodes the specified Core message. Does not implicitly {@link pb.CPUInfo.Core.verify|verify} messages.
             * @param message Core message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            public static encode(message: pb.CPUInfo.ICore, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified Core message, length delimited. Does not implicitly {@link pb.CPUInfo.Core.verify|verify} messages.
             * @param message Core message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            public static encodeDelimited(message: pb.CPUInfo.ICore, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a Core message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns Core
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.CPUInfo.Core;

            /**
             * Decodes a Core message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns Core
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.CPUInfo.Core;

            /**
             * Verifies a Core message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            public static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a Core message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns Core
             */
            public static fromObject(object: { [k: string]: any }): pb.CPUInfo.Core;

            /**
             * Creates a plain object from a Core message. Also converts values to other types if specified.
             * @param message Core
             * @param [options] Conversion options
             * @returns Plain object
             */
            public static toObject(message: pb.CPUInfo.Core, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this Core to JSON.
             * @returns JSON object
             */
            public toJSON(): { [k: string]: any };

            /**
             * Gets the default type url for Core
             * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
             * @returns The default type url
             */
            public static getTypeUrl(typeUrlPrefix?: string): string;
        }

        namespace Core {

            /** Type enum. */
            enum Type {
                PERFORMANCE = 0,
                EFFICIENCY = 1,
                HYPERTHREAD = 2
            }
        }
    }

    /** Properties of a MemoryInfo. */
    interface IMemoryInfo {

        /** MemoryInfo total */
        total?: (number|Long|null);

        /** MemoryInfo used */
        used?: (number|Long|null);

        /** MemoryInfo percent */
        percent?: (number|null);

        /** MemoryInfo cached */
        cached?: (number|Long|null);

        /** MemoryInfo buffers */
        buffers?: (number|Long|null);

        /** MemoryInfo shared */
        shared?: (number|Long|null);

        /** MemoryInfo zramUsed */
        zramUsed?: (number|Long|null);

        /** MemoryInfo zramTotal */
        zramTotal?: (number|Long|null);
    }

    /** Represents a MemoryInfo. */
    class MemoryInfo implements IMemoryInfo {

        /**
         * Constructs a new MemoryInfo.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IMemoryInfo);

        /** MemoryInfo total. */
        public total: (number|Long);

        /** MemoryInfo used. */
        public used: (number|Long);

        /** MemoryInfo percent. */
        public percent: number;

        /** MemoryInfo cached. */
        public cached: (number|Long);

        /** MemoryInfo buffers. */
        public buffers: (number|Long);

        /** MemoryInfo shared. */
        public shared: (number|Long);

        /** MemoryInfo zramUsed. */
        public zramUsed: (number|Long);

        /** MemoryInfo zramTotal. */
        public zramTotal: (number|Long);

        /**
         * Creates a new MemoryInfo instance using the specified properties.
         * @param [properties] Properties to set
         * @returns MemoryInfo instance
         */
        public static create(properties?: pb.IMemoryInfo): pb.MemoryInfo;

        /**
         * Encodes the specified MemoryInfo message. Does not implicitly {@link pb.MemoryInfo.verify|verify} messages.
         * @param message MemoryInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IMemoryInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified MemoryInfo message, length delimited. Does not implicitly {@link pb.MemoryInfo.verify|verify} messages.
         * @param message MemoryInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IMemoryInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a MemoryInfo message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns MemoryInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.MemoryInfo;

        /**
         * Decodes a MemoryInfo message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns MemoryInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.MemoryInfo;

        /**
         * Verifies a MemoryInfo message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a MemoryInfo message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns MemoryInfo
         */
        public static fromObject(object: { [k: string]: any }): pb.MemoryInfo;

        /**
         * Creates a plain object from a MemoryInfo message. Also converts values to other types if specified.
         * @param message MemoryInfo
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.MemoryInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this MemoryInfo to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for MemoryInfo
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a Hook. */
    interface IHook {

        /** Hook id */
        id?: (string|null);

        /** Hook name */
        name?: (string|null);

        /** Hook description */
        description?: (string|null);

        /** Hook installed */
        installed?: (boolean|null);

        /** Hook targetCmd */
        targetCmd?: (string|null);
    }

    /** Represents a Hook. */
    class Hook implements IHook {

        /**
         * Constructs a new Hook.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IHook);

        /** Hook id. */
        public id: string;

        /** Hook name. */
        public name: string;

        /** Hook description. */
        public description: string;

        /** Hook installed. */
        public installed: boolean;

        /** Hook targetCmd. */
        public targetCmd: string;

        /**
         * Creates a new Hook instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Hook instance
         */
        public static create(properties?: pb.IHook): pb.Hook;

        /**
         * Encodes the specified Hook message. Does not implicitly {@link pb.Hook.verify|verify} messages.
         * @param message Hook message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IHook, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Hook message, length delimited. Does not implicitly {@link pb.Hook.verify|verify} messages.
         * @param message Hook message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IHook, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a Hook message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Hook
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.Hook;

        /**
         * Decodes a Hook message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Hook
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.Hook;

        /**
         * Verifies a Hook message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a Hook message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Hook
         */
        public static fromObject(object: { [k: string]: any }): pb.Hook;

        /**
         * Creates a plain object from a Hook message. Also converts values to other types if specified.
         * @param message Hook
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.Hook, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Hook to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Hook
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a HookRequest. */
    interface IHookRequest {

        /** HookRequest id */
        id?: (string|null);

        /** HookRequest install */
        install?: (boolean|null);
    }

    /** Represents a HookRequest. */
    class HookRequest implements IHookRequest {

        /**
         * Constructs a new HookRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IHookRequest);

        /** HookRequest id. */
        public id: string;

        /** HookRequest install. */
        public install: boolean;

        /**
         * Creates a new HookRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns HookRequest instance
         */
        public static create(properties?: pb.IHookRequest): pb.HookRequest;

        /**
         * Encodes the specified HookRequest message. Does not implicitly {@link pb.HookRequest.verify|verify} messages.
         * @param message HookRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IHookRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified HookRequest message, length delimited. Does not implicitly {@link pb.HookRequest.verify|verify} messages.
         * @param message HookRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IHookRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a HookRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns HookRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.HookRequest;

        /**
         * Decodes a HookRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns HookRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.HookRequest;

        /**
         * Verifies a HookRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a HookRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns HookRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.HookRequest;

        /**
         * Creates a plain object from a HookRequest message. Also converts values to other types if specified.
         * @param message HookRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.HookRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this HookRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for HookRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a HookResponse. */
    interface IHookResponse {

        /** HookResponse success */
        success?: (boolean|null);

        /** HookResponse message */
        message?: (string|null);
    }

    /** Represents a HookResponse. */
    class HookResponse implements IHookResponse {

        /**
         * Constructs a new HookResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IHookResponse);

        /** HookResponse success. */
        public success: boolean;

        /** HookResponse message. */
        public message: string;

        /**
         * Creates a new HookResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns HookResponse instance
         */
        public static create(properties?: pb.IHookResponse): pb.HookResponse;

        /**
         * Encodes the specified HookResponse message. Does not implicitly {@link pb.HookResponse.verify|verify} messages.
         * @param message HookResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IHookResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified HookResponse message, length delimited. Does not implicitly {@link pb.HookResponse.verify|verify} messages.
         * @param message HookResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IHookResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a HookResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns HookResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.HookResponse;

        /**
         * Decodes a HookResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns HookResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.HookResponse;

        /**
         * Verifies a HookResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a HookResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns HookResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.HookResponse;

        /**
         * Creates a plain object from a HookResponse message. Also converts values to other types if specified.
         * @param message HookResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.HookResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this HookResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for HookResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a NetworkInterface. */
    interface INetworkInterface {

        /** NetworkInterface name */
        name?: (string|null);

        /** NetworkInterface recvBytes */
        recvBytes?: (number|Long|null);

        /** NetworkInterface sentBytes */
        sentBytes?: (number|Long|null);
    }

    /** Represents a NetworkInterface. */
    class NetworkInterface implements INetworkInterface {

        /**
         * Constructs a new NetworkInterface.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.INetworkInterface);

        /** NetworkInterface name. */
        public name: string;

        /** NetworkInterface recvBytes. */
        public recvBytes: (number|Long);

        /** NetworkInterface sentBytes. */
        public sentBytes: (number|Long);

        /**
         * Creates a new NetworkInterface instance using the specified properties.
         * @param [properties] Properties to set
         * @returns NetworkInterface instance
         */
        public static create(properties?: pb.INetworkInterface): pb.NetworkInterface;

        /**
         * Encodes the specified NetworkInterface message. Does not implicitly {@link pb.NetworkInterface.verify|verify} messages.
         * @param message NetworkInterface message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.INetworkInterface, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified NetworkInterface message, length delimited. Does not implicitly {@link pb.NetworkInterface.verify|verify} messages.
         * @param message NetworkInterface message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.INetworkInterface, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a NetworkInterface message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns NetworkInterface
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.NetworkInterface;

        /**
         * Decodes a NetworkInterface message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns NetworkInterface
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.NetworkInterface;

        /**
         * Verifies a NetworkInterface message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a NetworkInterface message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns NetworkInterface
         */
        public static fromObject(object: { [k: string]: any }): pb.NetworkInterface;

        /**
         * Creates a plain object from a NetworkInterface message. Also converts values to other types if specified.
         * @param message NetworkInterface
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.NetworkInterface, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this NetworkInterface to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for NetworkInterface
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a DiskDevice. */
    interface IDiskDevice {

        /** DiskDevice name */
        name?: (string|null);

        /** DiskDevice readBytes */
        readBytes?: (number|Long|null);

        /** DiskDevice writeBytes */
        writeBytes?: (number|Long|null);
    }

    /** Represents a DiskDevice. */
    class DiskDevice implements IDiskDevice {

        /**
         * Constructs a new DiskDevice.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IDiskDevice);

        /** DiskDevice name. */
        public name: string;

        /** DiskDevice readBytes. */
        public readBytes: (number|Long);

        /** DiskDevice writeBytes. */
        public writeBytes: (number|Long);

        /**
         * Creates a new DiskDevice instance using the specified properties.
         * @param [properties] Properties to set
         * @returns DiskDevice instance
         */
        public static create(properties?: pb.IDiskDevice): pb.DiskDevice;

        /**
         * Encodes the specified DiskDevice message. Does not implicitly {@link pb.DiskDevice.verify|verify} messages.
         * @param message DiskDevice message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IDiskDevice, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified DiskDevice message, length delimited. Does not implicitly {@link pb.DiskDevice.verify|verify} messages.
         * @param message DiskDevice message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IDiskDevice, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a DiskDevice message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns DiskDevice
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.DiskDevice;

        /**
         * Decodes a DiskDevice message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns DiskDevice
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.DiskDevice;

        /**
         * Verifies a DiskDevice message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a DiskDevice message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns DiskDevice
         */
        public static fromObject(object: { [k: string]: any }): pb.DiskDevice;

        /**
         * Creates a plain object from a DiskDevice message. Also converts values to other types if specified.
         * @param message DiskDevice
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.DiskDevice, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this DiskDevice to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for DiskDevice
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a IOInfo. */
    interface IIOInfo {

        /** IOInfo totalReadBytes */
        totalReadBytes?: (number|Long|null);

        /** IOInfo totalWriteBytes */
        totalWriteBytes?: (number|Long|null);

        /** IOInfo totalNetRecvBytes */
        totalNetRecvBytes?: (number|Long|null);

        /** IOInfo totalNetSentBytes */
        totalNetSentBytes?: (number|Long|null);

        /** IOInfo networks */
        networks?: (pb.INetworkInterface[]|null);

        /** IOInfo disks */
        disks?: (pb.IDiskDevice[]|null);
    }

    /** Represents a IOInfo. */
    class IOInfo implements IIOInfo {

        /**
         * Constructs a new IOInfo.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IIOInfo);

        /** IOInfo totalReadBytes. */
        public totalReadBytes: (number|Long);

        /** IOInfo totalWriteBytes. */
        public totalWriteBytes: (number|Long);

        /** IOInfo totalNetRecvBytes. */
        public totalNetRecvBytes: (number|Long);

        /** IOInfo totalNetSentBytes. */
        public totalNetSentBytes: (number|Long);

        /** IOInfo networks. */
        public networks: pb.INetworkInterface[];

        /** IOInfo disks. */
        public disks: pb.IDiskDevice[];

        /**
         * Creates a new IOInfo instance using the specified properties.
         * @param [properties] Properties to set
         * @returns IOInfo instance
         */
        public static create(properties?: pb.IIOInfo): pb.IOInfo;

        /**
         * Encodes the specified IOInfo message. Does not implicitly {@link pb.IOInfo.verify|verify} messages.
         * @param message IOInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IIOInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified IOInfo message, length delimited. Does not implicitly {@link pb.IOInfo.verify|verify} messages.
         * @param message IOInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IIOInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a IOInfo message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns IOInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.IOInfo;

        /**
         * Decodes a IOInfo message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns IOInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.IOInfo;

        /**
         * Verifies a IOInfo message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a IOInfo message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns IOInfo
         */
        public static fromObject(object: { [k: string]: any }): pb.IOInfo;

        /**
         * Creates a plain object from a IOInfo message. Also converts values to other types if specified.
         * @param message IOInfo
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.IOInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this IOInfo to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for IOInfo
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a FaultInfo. */
    interface IFaultInfo {

        /** FaultInfo pageFaults */
        pageFaults?: (number|Long|null);

        /** FaultInfo majorFaults */
        majorFaults?: (number|Long|null);

        /** FaultInfo minorFaults */
        minorFaults?: (number|Long|null);

        /** FaultInfo pageFaultRate */
        pageFaultRate?: (number|null);

        /** FaultInfo majorFaultRate */
        majorFaultRate?: (number|null);

        /** FaultInfo minorFaultRate */
        minorFaultRate?: (number|null);

        /** FaultInfo swapIn */
        swapIn?: (number|Long|null);

        /** FaultInfo swapOut */
        swapOut?: (number|Long|null);

        /** FaultInfo swapInRate */
        swapInRate?: (number|null);

        /** FaultInfo swapOutRate */
        swapOutRate?: (number|null);
    }

    /** Represents a FaultInfo. */
    class FaultInfo implements IFaultInfo {

        /**
         * Constructs a new FaultInfo.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IFaultInfo);

        /** FaultInfo pageFaults. */
        public pageFaults: (number|Long);

        /** FaultInfo majorFaults. */
        public majorFaults: (number|Long);

        /** FaultInfo minorFaults. */
        public minorFaults: (number|Long);

        /** FaultInfo pageFaultRate. */
        public pageFaultRate: number;

        /** FaultInfo majorFaultRate. */
        public majorFaultRate: number;

        /** FaultInfo minorFaultRate. */
        public minorFaultRate: number;

        /** FaultInfo swapIn. */
        public swapIn: (number|Long);

        /** FaultInfo swapOut. */
        public swapOut: (number|Long);

        /** FaultInfo swapInRate. */
        public swapInRate: number;

        /** FaultInfo swapOutRate. */
        public swapOutRate: number;

        /**
         * Creates a new FaultInfo instance using the specified properties.
         * @param [properties] Properties to set
         * @returns FaultInfo instance
         */
        public static create(properties?: pb.IFaultInfo): pb.FaultInfo;

        /**
         * Encodes the specified FaultInfo message. Does not implicitly {@link pb.FaultInfo.verify|verify} messages.
         * @param message FaultInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IFaultInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified FaultInfo message, length delimited. Does not implicitly {@link pb.FaultInfo.verify|verify} messages.
         * @param message FaultInfo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IFaultInfo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a FaultInfo message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns FaultInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.FaultInfo;

        /**
         * Decodes a FaultInfo message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns FaultInfo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.FaultInfo;

        /**
         * Verifies a FaultInfo message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a FaultInfo message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns FaultInfo
         */
        public static fromObject(object: { [k: string]: any }): pb.FaultInfo;

        /**
         * Creates a plain object from a FaultInfo message. Also converts values to other types if specified.
         * @param message FaultInfo
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.FaultInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this FaultInfo to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for FaultInfo
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

        /** SystemStats cpu */
        cpu?: (pb.ICPUInfo|null);

        /** SystemStats memory */
        memory?: (pb.IMemoryInfo|null);

        /** SystemStats io */
        io?: (pb.IIOInfo|null);

        /** SystemStats faults */
        faults?: (pb.IFaultInfo|null);
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

        /** SystemStats cpu. */
        public cpu?: (pb.ICPUInfo|null);

        /** SystemStats memory. */
        public memory?: (pb.IMemoryInfo|null);

        /** SystemStats io. */
        public io?: (pb.IIOInfo|null);

        /** SystemStats faults. */
        public faults?: (pb.IFaultInfo|null);

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
