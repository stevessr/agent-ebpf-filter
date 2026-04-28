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

        /** GPUStatus encUtil */
        encUtil?: (number|null);

        /** GPUStatus decUtil */
        decUtil?: (number|null);

        /** GPUStatus smClockMhz */
        smClockMhz?: (number|null);

        /** GPUStatus memClockMhz */
        memClockMhz?: (number|null);

        /** GPUStatus gfxClockMhz */
        gfxClockMhz?: (number|null);

        /** GPUStatus powerW */
        powerW?: (number|null);

        /** GPUStatus powerLimitW */
        powerLimitW?: (number|null);

        /** GPUStatus fanSpeed */
        fanSpeed?: (number|null);

        /** GPUStatus pcieGen */
        pcieGen?: (number|null);

        /** GPUStatus pcieWidth */
        pcieWidth?: (number|null);
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

        /** GPUStatus encUtil. */
        public encUtil: number;

        /** GPUStatus decUtil. */
        public decUtil: number;

        /** GPUStatus smClockMhz. */
        public smClockMhz: number;

        /** GPUStatus memClockMhz. */
        public memClockMhz: number;

        /** GPUStatus gfxClockMhz. */
        public gfxClockMhz: number;

        /** GPUStatus powerW. */
        public powerW: number;

        /** GPUStatus powerLimitW. */
        public powerLimitW: number;

        /** GPUStatus fanSpeed. */
        public fanSpeed: number;

        /** GPUStatus pcieGen. */
        public pcieGen: number;

        /** GPUStatus pcieWidth. */
        public pcieWidth: number;

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

        /** MemoryInfo swapTotal */
        swapTotal?: (number|Long|null);

        /** MemoryInfo swapUsed */
        swapUsed?: (number|Long|null);
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

        /** MemoryInfo swapTotal. */
        public swapTotal: (number|Long);

        /** MemoryInfo swapUsed. */
        public swapUsed: (number|Long);

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

    /** Properties of a ConfigTag. */
    interface IConfigTag {

        /** ConfigTag name */
        name?: (string|null);
    }

    /** Represents a ConfigTag. */
    class ConfigTag implements IConfigTag {

        /**
         * Constructs a new ConfigTag.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IConfigTag);

        /** ConfigTag name. */
        public name: string;

        /**
         * Creates a new ConfigTag instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ConfigTag instance
         */
        public static create(properties?: pb.IConfigTag): pb.ConfigTag;

        /**
         * Encodes the specified ConfigTag message. Does not implicitly {@link pb.ConfigTag.verify|verify} messages.
         * @param message ConfigTag message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IConfigTag, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ConfigTag message, length delimited. Does not implicitly {@link pb.ConfigTag.verify|verify} messages.
         * @param message ConfigTag message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IConfigTag, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ConfigTag message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ConfigTag
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ConfigTag;

        /**
         * Decodes a ConfigTag message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ConfigTag
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ConfigTag;

        /**
         * Verifies a ConfigTag message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ConfigTag message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ConfigTag
         */
        public static fromObject(object: { [k: string]: any }): pb.ConfigTag;

        /**
         * Creates a plain object from a ConfigTag message. Also converts values to other types if specified.
         * @param message ConfigTag
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ConfigTag, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ConfigTag to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ConfigTag
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ConfigTagList. */
    interface IConfigTagList {

        /** ConfigTagList names */
        names?: (string[]|null);
    }

    /** Represents a ConfigTagList. */
    class ConfigTagList implements IConfigTagList {

        /**
         * Constructs a new ConfigTagList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IConfigTagList);

        /** ConfigTagList names. */
        public names: string[];

        /**
         * Creates a new ConfigTagList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ConfigTagList instance
         */
        public static create(properties?: pb.IConfigTagList): pb.ConfigTagList;

        /**
         * Encodes the specified ConfigTagList message. Does not implicitly {@link pb.ConfigTagList.verify|verify} messages.
         * @param message ConfigTagList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IConfigTagList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ConfigTagList message, length delimited. Does not implicitly {@link pb.ConfigTagList.verify|verify} messages.
         * @param message ConfigTagList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IConfigTagList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ConfigTagList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ConfigTagList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ConfigTagList;

        /**
         * Decodes a ConfigTagList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ConfigTagList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ConfigTagList;

        /**
         * Verifies a ConfigTagList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ConfigTagList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ConfigTagList
         */
        public static fromObject(object: { [k: string]: any }): pb.ConfigTagList;

        /**
         * Creates a plain object from a ConfigTagList message. Also converts values to other types if specified.
         * @param message ConfigTagList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ConfigTagList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ConfigTagList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ConfigTagList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedComm. */
    interface ITrackedComm {

        /** TrackedComm comm */
        comm?: (string|null);

        /** TrackedComm tag */
        tag?: (string|null);

        /** TrackedComm disabled */
        disabled?: (boolean|null);
    }

    /** Represents a TrackedComm. */
    class TrackedComm implements ITrackedComm {

        /**
         * Constructs a new TrackedComm.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedComm);

        /** TrackedComm comm. */
        public comm: string;

        /** TrackedComm tag. */
        public tag: string;

        /** TrackedComm disabled. */
        public disabled: boolean;

        /**
         * Creates a new TrackedComm instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedComm instance
         */
        public static create(properties?: pb.ITrackedComm): pb.TrackedComm;

        /**
         * Encodes the specified TrackedComm message. Does not implicitly {@link pb.TrackedComm.verify|verify} messages.
         * @param message TrackedComm message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedComm, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedComm message, length delimited. Does not implicitly {@link pb.TrackedComm.verify|verify} messages.
         * @param message TrackedComm message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedComm, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedComm message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedComm
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedComm;

        /**
         * Decodes a TrackedComm message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedComm
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedComm;

        /**
         * Verifies a TrackedComm message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedComm message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedComm
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedComm;

        /**
         * Creates a plain object from a TrackedComm message. Also converts values to other types if specified.
         * @param message TrackedComm
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedComm, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedComm to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedComm
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedCommList. */
    interface ITrackedCommList {

        /** TrackedCommList items */
        items?: (pb.ITrackedComm[]|null);
    }

    /** Represents a TrackedCommList. */
    class TrackedCommList implements ITrackedCommList {

        /**
         * Constructs a new TrackedCommList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedCommList);

        /** TrackedCommList items. */
        public items: pb.ITrackedComm[];

        /**
         * Creates a new TrackedCommList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedCommList instance
         */
        public static create(properties?: pb.ITrackedCommList): pb.TrackedCommList;

        /**
         * Encodes the specified TrackedCommList message. Does not implicitly {@link pb.TrackedCommList.verify|verify} messages.
         * @param message TrackedCommList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedCommList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedCommList message, length delimited. Does not implicitly {@link pb.TrackedCommList.verify|verify} messages.
         * @param message TrackedCommList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedCommList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedCommList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedCommList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedCommList;

        /**
         * Decodes a TrackedCommList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedCommList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedCommList;

        /**
         * Verifies a TrackedCommList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedCommList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedCommList
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedCommList;

        /**
         * Creates a plain object from a TrackedCommList message. Also converts values to other types if specified.
         * @param message TrackedCommList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedCommList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedCommList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedCommList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedPath. */
    interface ITrackedPath {

        /** TrackedPath path */
        path?: (string|null);

        /** TrackedPath tag */
        tag?: (string|null);
    }

    /** Represents a TrackedPath. */
    class TrackedPath implements ITrackedPath {

        /**
         * Constructs a new TrackedPath.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedPath);

        /** TrackedPath path. */
        public path: string;

        /** TrackedPath tag. */
        public tag: string;

        /**
         * Creates a new TrackedPath instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedPath instance
         */
        public static create(properties?: pb.ITrackedPath): pb.TrackedPath;

        /**
         * Encodes the specified TrackedPath message. Does not implicitly {@link pb.TrackedPath.verify|verify} messages.
         * @param message TrackedPath message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedPath, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedPath message, length delimited. Does not implicitly {@link pb.TrackedPath.verify|verify} messages.
         * @param message TrackedPath message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedPath, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedPath message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedPath
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedPath;

        /**
         * Decodes a TrackedPath message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedPath
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedPath;

        /**
         * Verifies a TrackedPath message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedPath message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedPath
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedPath;

        /**
         * Creates a plain object from a TrackedPath message. Also converts values to other types if specified.
         * @param message TrackedPath
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedPath, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedPath to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedPath
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedPathList. */
    interface ITrackedPathList {

        /** TrackedPathList items */
        items?: (pb.ITrackedPath[]|null);
    }

    /** Represents a TrackedPathList. */
    class TrackedPathList implements ITrackedPathList {

        /**
         * Constructs a new TrackedPathList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedPathList);

        /** TrackedPathList items. */
        public items: pb.ITrackedPath[];

        /**
         * Creates a new TrackedPathList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedPathList instance
         */
        public static create(properties?: pb.ITrackedPathList): pb.TrackedPathList;

        /**
         * Encodes the specified TrackedPathList message. Does not implicitly {@link pb.TrackedPathList.verify|verify} messages.
         * @param message TrackedPathList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedPathList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedPathList message, length delimited. Does not implicitly {@link pb.TrackedPathList.verify|verify} messages.
         * @param message TrackedPathList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedPathList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedPathList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedPathList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedPathList;

        /**
         * Decodes a TrackedPathList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedPathList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedPathList;

        /**
         * Verifies a TrackedPathList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedPathList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedPathList
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedPathList;

        /**
         * Creates a plain object from a TrackedPathList message. Also converts values to other types if specified.
         * @param message TrackedPathList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedPathList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedPathList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedPathList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedPrefix. */
    interface ITrackedPrefix {

        /** TrackedPrefix prefix */
        prefix?: (string|null);

        /** TrackedPrefix tag */
        tag?: (string|null);
    }

    /** Represents a TrackedPrefix. */
    class TrackedPrefix implements ITrackedPrefix {

        /**
         * Constructs a new TrackedPrefix.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedPrefix);

        /** TrackedPrefix prefix. */
        public prefix: string;

        /** TrackedPrefix tag. */
        public tag: string;

        /**
         * Creates a new TrackedPrefix instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedPrefix instance
         */
        public static create(properties?: pb.ITrackedPrefix): pb.TrackedPrefix;

        /**
         * Encodes the specified TrackedPrefix message. Does not implicitly {@link pb.TrackedPrefix.verify|verify} messages.
         * @param message TrackedPrefix message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedPrefix, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedPrefix message, length delimited. Does not implicitly {@link pb.TrackedPrefix.verify|verify} messages.
         * @param message TrackedPrefix message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedPrefix, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedPrefix message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedPrefix
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedPrefix;

        /**
         * Decodes a TrackedPrefix message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedPrefix
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedPrefix;

        /**
         * Verifies a TrackedPrefix message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedPrefix message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedPrefix
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedPrefix;

        /**
         * Creates a plain object from a TrackedPrefix message. Also converts values to other types if specified.
         * @param message TrackedPrefix
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedPrefix, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedPrefix to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedPrefix
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a TrackedPrefixList. */
    interface ITrackedPrefixList {

        /** TrackedPrefixList items */
        items?: (pb.ITrackedPrefix[]|null);
    }

    /** Represents a TrackedPrefixList. */
    class TrackedPrefixList implements ITrackedPrefixList {

        /**
         * Constructs a new TrackedPrefixList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ITrackedPrefixList);

        /** TrackedPrefixList items. */
        public items: pb.ITrackedPrefix[];

        /**
         * Creates a new TrackedPrefixList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns TrackedPrefixList instance
         */
        public static create(properties?: pb.ITrackedPrefixList): pb.TrackedPrefixList;

        /**
         * Encodes the specified TrackedPrefixList message. Does not implicitly {@link pb.TrackedPrefixList.verify|verify} messages.
         * @param message TrackedPrefixList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ITrackedPrefixList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified TrackedPrefixList message, length delimited. Does not implicitly {@link pb.TrackedPrefixList.verify|verify} messages.
         * @param message TrackedPrefixList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ITrackedPrefixList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a TrackedPrefixList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns TrackedPrefixList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.TrackedPrefixList;

        /**
         * Decodes a TrackedPrefixList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns TrackedPrefixList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.TrackedPrefixList;

        /**
         * Verifies a TrackedPrefixList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a TrackedPrefixList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns TrackedPrefixList
         */
        public static fromObject(object: { [k: string]: any }): pb.TrackedPrefixList;

        /**
         * Creates a plain object from a TrackedPrefixList message. Also converts values to other types if specified.
         * @param message TrackedPrefixList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.TrackedPrefixList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this TrackedPrefixList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for TrackedPrefixList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a WrapperRule. */
    interface IWrapperRule {

        /** WrapperRule comm */
        comm?: (string|null);

        /** WrapperRule action */
        action?: (string|null);

        /** WrapperRule rewrittenCmd */
        rewrittenCmd?: (string[]|null);

        /** WrapperRule regex */
        regex?: (string|null);

        /** WrapperRule replacement */
        replacement?: (string|null);

        /** WrapperRule priority */
        priority?: (number|null);
    }

    /** Represents a WrapperRule. */
    class WrapperRule implements IWrapperRule {

        /**
         * Constructs a new WrapperRule.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IWrapperRule);

        /** WrapperRule comm. */
        public comm: string;

        /** WrapperRule action. */
        public action: string;

        /** WrapperRule rewrittenCmd. */
        public rewrittenCmd: string[];

        /** WrapperRule regex. */
        public regex: string;

        /** WrapperRule replacement. */
        public replacement: string;

        /** WrapperRule priority. */
        public priority: number;

        /**
         * Creates a new WrapperRule instance using the specified properties.
         * @param [properties] Properties to set
         * @returns WrapperRule instance
         */
        public static create(properties?: pb.IWrapperRule): pb.WrapperRule;

        /**
         * Encodes the specified WrapperRule message. Does not implicitly {@link pb.WrapperRule.verify|verify} messages.
         * @param message WrapperRule message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IWrapperRule, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified WrapperRule message, length delimited. Does not implicitly {@link pb.WrapperRule.verify|verify} messages.
         * @param message WrapperRule message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IWrapperRule, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a WrapperRule message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns WrapperRule
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.WrapperRule;

        /**
         * Decodes a WrapperRule message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns WrapperRule
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.WrapperRule;

        /**
         * Verifies a WrapperRule message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a WrapperRule message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns WrapperRule
         */
        public static fromObject(object: { [k: string]: any }): pb.WrapperRule;

        /**
         * Creates a plain object from a WrapperRule message. Also converts values to other types if specified.
         * @param message WrapperRule
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.WrapperRule, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this WrapperRule to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for WrapperRule
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a WrapperRuleList. */
    interface IWrapperRuleList {

        /** WrapperRuleList items */
        items?: (pb.IWrapperRule[]|null);
    }

    /** Represents a WrapperRuleList. */
    class WrapperRuleList implements IWrapperRuleList {

        /**
         * Constructs a new WrapperRuleList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IWrapperRuleList);

        /** WrapperRuleList items. */
        public items: pb.IWrapperRule[];

        /**
         * Creates a new WrapperRuleList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns WrapperRuleList instance
         */
        public static create(properties?: pb.IWrapperRuleList): pb.WrapperRuleList;

        /**
         * Encodes the specified WrapperRuleList message. Does not implicitly {@link pb.WrapperRuleList.verify|verify} messages.
         * @param message WrapperRuleList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IWrapperRuleList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified WrapperRuleList message, length delimited. Does not implicitly {@link pb.WrapperRuleList.verify|verify} messages.
         * @param message WrapperRuleList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IWrapperRuleList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a WrapperRuleList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns WrapperRuleList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.WrapperRuleList;

        /**
         * Decodes a WrapperRuleList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns WrapperRuleList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.WrapperRuleList;

        /**
         * Verifies a WrapperRuleList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a WrapperRuleList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns WrapperRuleList
         */
        public static fromObject(object: { [k: string]: any }): pb.WrapperRuleList;

        /**
         * Creates a plain object from a WrapperRuleList message. Also converts values to other types if specified.
         * @param message WrapperRuleList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.WrapperRuleList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this WrapperRuleList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for WrapperRuleList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a RuntimeSettings. */
    interface IRuntimeSettings {

        /** RuntimeSettings logPersistenceEnabled */
        logPersistenceEnabled?: (boolean|null);

        /** RuntimeSettings logFilePath */
        logFilePath?: (string|null);

        /** RuntimeSettings accessToken */
        accessToken?: (string|null);

        /** RuntimeSettings maxEventCount */
        maxEventCount?: (number|null);

        /** RuntimeSettings maxEventAge */
        maxEventAge?: (string|null);
    }

    /** Represents a RuntimeSettings. */
    class RuntimeSettings implements IRuntimeSettings {

        /**
         * Constructs a new RuntimeSettings.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRuntimeSettings);

        /** RuntimeSettings logPersistenceEnabled. */
        public logPersistenceEnabled: boolean;

        /** RuntimeSettings logFilePath. */
        public logFilePath: string;

        /** RuntimeSettings accessToken. */
        public accessToken: string;

        /** RuntimeSettings maxEventCount. */
        public maxEventCount: number;

        /** RuntimeSettings maxEventAge. */
        public maxEventAge: string;

        /**
         * Creates a new RuntimeSettings instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RuntimeSettings instance
         */
        public static create(properties?: pb.IRuntimeSettings): pb.RuntimeSettings;

        /**
         * Encodes the specified RuntimeSettings message. Does not implicitly {@link pb.RuntimeSettings.verify|verify} messages.
         * @param message RuntimeSettings message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRuntimeSettings, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RuntimeSettings message, length delimited. Does not implicitly {@link pb.RuntimeSettings.verify|verify} messages.
         * @param message RuntimeSettings message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRuntimeSettings, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RuntimeSettings message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RuntimeSettings
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RuntimeSettings;

        /**
         * Decodes a RuntimeSettings message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RuntimeSettings
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RuntimeSettings;

        /**
         * Verifies a RuntimeSettings message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RuntimeSettings message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RuntimeSettings
         */
        public static fromObject(object: { [k: string]: any }): pb.RuntimeSettings;

        /**
         * Creates a plain object from a RuntimeSettings message. Also converts values to other types if specified.
         * @param message RuntimeSettings
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RuntimeSettings, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RuntimeSettings to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RuntimeSettings
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a RuntimeConfigResponse. */
    interface IRuntimeConfigResponse {

        /** RuntimeConfigResponse runtime */
        runtime?: (pb.IRuntimeSettings|null);

        /** RuntimeConfigResponse mcpEndpoint */
        mcpEndpoint?: (string|null);

        /** RuntimeConfigResponse authHeaderName */
        authHeaderName?: (string|null);

        /** RuntimeConfigResponse bearerAuthHeaderName */
        bearerAuthHeaderName?: (string|null);

        /** RuntimeConfigResponse persistedEventLogPath */
        persistedEventLogPath?: (string|null);

        /** RuntimeConfigResponse persistedEventLogAlive */
        persistedEventLogAlive?: (boolean|null);
    }

    /** Represents a RuntimeConfigResponse. */
    class RuntimeConfigResponse implements IRuntimeConfigResponse {

        /**
         * Constructs a new RuntimeConfigResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRuntimeConfigResponse);

        /** RuntimeConfigResponse runtime. */
        public runtime?: (pb.IRuntimeSettings|null);

        /** RuntimeConfigResponse mcpEndpoint. */
        public mcpEndpoint: string;

        /** RuntimeConfigResponse authHeaderName. */
        public authHeaderName: string;

        /** RuntimeConfigResponse bearerAuthHeaderName. */
        public bearerAuthHeaderName: string;

        /** RuntimeConfigResponse persistedEventLogPath. */
        public persistedEventLogPath: string;

        /** RuntimeConfigResponse persistedEventLogAlive. */
        public persistedEventLogAlive: boolean;

        /**
         * Creates a new RuntimeConfigResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RuntimeConfigResponse instance
         */
        public static create(properties?: pb.IRuntimeConfigResponse): pb.RuntimeConfigResponse;

        /**
         * Encodes the specified RuntimeConfigResponse message. Does not implicitly {@link pb.RuntimeConfigResponse.verify|verify} messages.
         * @param message RuntimeConfigResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRuntimeConfigResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RuntimeConfigResponse message, length delimited. Does not implicitly {@link pb.RuntimeConfigResponse.verify|verify} messages.
         * @param message RuntimeConfigResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRuntimeConfigResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RuntimeConfigResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RuntimeConfigResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RuntimeConfigResponse;

        /**
         * Decodes a RuntimeConfigResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RuntimeConfigResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RuntimeConfigResponse;

        /**
         * Verifies a RuntimeConfigResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RuntimeConfigResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RuntimeConfigResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.RuntimeConfigResponse;

        /**
         * Creates a plain object from a RuntimeConfigResponse message. Also converts values to other types if specified.
         * @param message RuntimeConfigResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RuntimeConfigResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RuntimeConfigResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RuntimeConfigResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an ExportConfigData. */
    interface IExportConfigData {

        /** ExportConfigData tags */
        tags?: (string[]|null);

        /** ExportConfigData comms */
        comms?: (pb.ITrackedComm[]|null);

        /** ExportConfigData paths */
        paths?: (pb.ITrackedPath[]|null);

        /** ExportConfigData prefixes */
        prefixes?: (pb.ITrackedPrefix[]|null);

        /** ExportConfigData rules */
        rules?: (pb.IWrapperRule[]|null);

        /** ExportConfigData runtime */
        runtime?: (pb.IRuntimeSettings|null);
    }

    /** Represents an ExportConfigData. */
    class ExportConfigData implements IExportConfigData {

        /**
         * Constructs a new ExportConfigData.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IExportConfigData);

        /** ExportConfigData tags. */
        public tags: string[];

        /** ExportConfigData comms. */
        public comms: pb.ITrackedComm[];

        /** ExportConfigData paths. */
        public paths: pb.ITrackedPath[];

        /** ExportConfigData prefixes. */
        public prefixes: pb.ITrackedPrefix[];

        /** ExportConfigData rules. */
        public rules: pb.IWrapperRule[];

        /** ExportConfigData runtime. */
        public runtime?: (pb.IRuntimeSettings|null);

        /**
         * Creates a new ExportConfigData instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ExportConfigData instance
         */
        public static create(properties?: pb.IExportConfigData): pb.ExportConfigData;

        /**
         * Encodes the specified ExportConfigData message. Does not implicitly {@link pb.ExportConfigData.verify|verify} messages.
         * @param message ExportConfigData message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IExportConfigData, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ExportConfigData message, length delimited. Does not implicitly {@link pb.ExportConfigData.verify|verify} messages.
         * @param message ExportConfigData message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IExportConfigData, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an ExportConfigData message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ExportConfigData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ExportConfigData;

        /**
         * Decodes an ExportConfigData message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ExportConfigData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ExportConfigData;

        /**
         * Verifies an ExportConfigData message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an ExportConfigData message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ExportConfigData
         */
        public static fromObject(object: { [k: string]: any }): pb.ExportConfigData;

        /**
         * Creates a plain object from an ExportConfigData message. Also converts values to other types if specified.
         * @param message ExportConfigData
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ExportConfigData, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ExportConfigData to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ExportConfigData
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a StatusResponse. */
    interface IStatusResponse {

        /** StatusResponse status */
        status?: (string|null);

        /** StatusResponse message */
        message?: (string|null);
    }

    /** Represents a StatusResponse. */
    class StatusResponse implements IStatusResponse {

        /**
         * Constructs a new StatusResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IStatusResponse);

        /** StatusResponse status. */
        public status: string;

        /** StatusResponse message. */
        public message: string;

        /**
         * Creates a new StatusResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns StatusResponse instance
         */
        public static create(properties?: pb.IStatusResponse): pb.StatusResponse;

        /**
         * Encodes the specified StatusResponse message. Does not implicitly {@link pb.StatusResponse.verify|verify} messages.
         * @param message StatusResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IStatusResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified StatusResponse message, length delimited. Does not implicitly {@link pb.StatusResponse.verify|verify} messages.
         * @param message StatusResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IStatusResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a StatusResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns StatusResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.StatusResponse;

        /**
         * Decodes a StatusResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns StatusResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.StatusResponse;

        /**
         * Verifies a StatusResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a StatusResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns StatusResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.StatusResponse;

        /**
         * Creates a plain object from a StatusResponse message. Also converts values to other types if specified.
         * @param message StatusResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.StatusResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this StatusResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for StatusResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a FileEntry. */
    interface IFileEntry {

        /** FileEntry name */
        name?: (string|null);

        /** FileEntry isDir */
        isDir?: (boolean|null);

        /** FileEntry path */
        path?: (string|null);

        /** FileEntry mimeType */
        mimeType?: (string|null);

        /** FileEntry size */
        size?: (number|Long|null);

        /** FileEntry modTime */
        modTime?: (string|null);
    }

    /** Represents a FileEntry. */
    class FileEntry implements IFileEntry {

        /**
         * Constructs a new FileEntry.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IFileEntry);

        /** FileEntry name. */
        public name: string;

        /** FileEntry isDir. */
        public isDir: boolean;

        /** FileEntry path. */
        public path: string;

        /** FileEntry mimeType. */
        public mimeType: string;

        /** FileEntry size. */
        public size: (number|Long);

        /** FileEntry modTime. */
        public modTime: string;

        /**
         * Creates a new FileEntry instance using the specified properties.
         * @param [properties] Properties to set
         * @returns FileEntry instance
         */
        public static create(properties?: pb.IFileEntry): pb.FileEntry;

        /**
         * Encodes the specified FileEntry message. Does not implicitly {@link pb.FileEntry.verify|verify} messages.
         * @param message FileEntry message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IFileEntry, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified FileEntry message, length delimited. Does not implicitly {@link pb.FileEntry.verify|verify} messages.
         * @param message FileEntry message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IFileEntry, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a FileEntry message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns FileEntry
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.FileEntry;

        /**
         * Decodes a FileEntry message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns FileEntry
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.FileEntry;

        /**
         * Verifies a FileEntry message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a FileEntry message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns FileEntry
         */
        public static fromObject(object: { [k: string]: any }): pb.FileEntry;

        /**
         * Creates a plain object from a FileEntry message. Also converts values to other types if specified.
         * @param message FileEntry
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.FileEntry, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this FileEntry to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for FileEntry
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a FileListResponse. */
    interface IFileListResponse {

        /** FileListResponse items */
        items?: (pb.IFileEntry[]|null);

        /** FileListResponse total */
        total?: (number|null);

        /** FileListResponse offset */
        offset?: (number|null);

        /** FileListResponse limit */
        limit?: (number|null);
    }

    /** Represents a FileListResponse. */
    class FileListResponse implements IFileListResponse {

        /**
         * Constructs a new FileListResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IFileListResponse);

        /** FileListResponse items. */
        public items: pb.IFileEntry[];

        /** FileListResponse total. */
        public total: number;

        /** FileListResponse offset. */
        public offset: number;

        /** FileListResponse limit. */
        public limit: number;

        /**
         * Creates a new FileListResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns FileListResponse instance
         */
        public static create(properties?: pb.IFileListResponse): pb.FileListResponse;

        /**
         * Encodes the specified FileListResponse message. Does not implicitly {@link pb.FileListResponse.verify|verify} messages.
         * @param message FileListResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IFileListResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified FileListResponse message, length delimited. Does not implicitly {@link pb.FileListResponse.verify|verify} messages.
         * @param message FileListResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IFileListResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a FileListResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns FileListResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.FileListResponse;

        /**
         * Decodes a FileListResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns FileListResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.FileListResponse;

        /**
         * Verifies a FileListResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a FileListResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns FileListResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.FileListResponse;

        /**
         * Creates a plain object from a FileListResponse message. Also converts values to other types if specified.
         * @param message FileListResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.FileListResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this FileListResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for FileListResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a FilePreviewResponse. */
    interface IFilePreviewResponse {

        /** FilePreviewResponse content */
        content?: (string|null);

        /** FilePreviewResponse mimeType */
        mimeType?: (string|null);

        /** FilePreviewResponse path */
        path?: (string|null);

        /** FilePreviewResponse size */
        size?: (number|Long|null);

        /** FilePreviewResponse modTime */
        modTime?: (string|null);

        /** FilePreviewResponse truncated */
        truncated?: (boolean|null);
    }

    /** Represents a FilePreviewResponse. */
    class FilePreviewResponse implements IFilePreviewResponse {

        /**
         * Constructs a new FilePreviewResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IFilePreviewResponse);

        /** FilePreviewResponse content. */
        public content: string;

        /** FilePreviewResponse mimeType. */
        public mimeType: string;

        /** FilePreviewResponse path. */
        public path: string;

        /** FilePreviewResponse size. */
        public size: (number|Long);

        /** FilePreviewResponse modTime. */
        public modTime: string;

        /** FilePreviewResponse truncated. */
        public truncated: boolean;

        /**
         * Creates a new FilePreviewResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns FilePreviewResponse instance
         */
        public static create(properties?: pb.IFilePreviewResponse): pb.FilePreviewResponse;

        /**
         * Encodes the specified FilePreviewResponse message. Does not implicitly {@link pb.FilePreviewResponse.verify|verify} messages.
         * @param message FilePreviewResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IFilePreviewResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified FilePreviewResponse message, length delimited. Does not implicitly {@link pb.FilePreviewResponse.verify|verify} messages.
         * @param message FilePreviewResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IFilePreviewResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a FilePreviewResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns FilePreviewResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.FilePreviewResponse;

        /**
         * Decodes a FilePreviewResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns FilePreviewResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.FilePreviewResponse;

        /**
         * Verifies a FilePreviewResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a FilePreviewResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns FilePreviewResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.FilePreviewResponse;

        /**
         * Creates a plain object from a FilePreviewResponse message. Also converts values to other types if specified.
         * @param message FilePreviewResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.FilePreviewResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this FilePreviewResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for FilePreviewResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a SystemdUnit. */
    interface ISystemdUnit {

        /** SystemdUnit unit */
        unit?: (string|null);

        /** SystemdUnit load */
        load?: (string|null);

        /** SystemdUnit active */
        active?: (string|null);

        /** SystemdUnit sub */
        sub?: (string|null);

        /** SystemdUnit description */
        description?: (string|null);
    }

    /** Represents a SystemdUnit. */
    class SystemdUnit implements ISystemdUnit {

        /**
         * Constructs a new SystemdUnit.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ISystemdUnit);

        /** SystemdUnit unit. */
        public unit: string;

        /** SystemdUnit load. */
        public load: string;

        /** SystemdUnit active. */
        public active: string;

        /** SystemdUnit sub. */
        public sub: string;

        /** SystemdUnit description. */
        public description: string;

        /**
         * Creates a new SystemdUnit instance using the specified properties.
         * @param [properties] Properties to set
         * @returns SystemdUnit instance
         */
        public static create(properties?: pb.ISystemdUnit): pb.SystemdUnit;

        /**
         * Encodes the specified SystemdUnit message. Does not implicitly {@link pb.SystemdUnit.verify|verify} messages.
         * @param message SystemdUnit message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ISystemdUnit, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified SystemdUnit message, length delimited. Does not implicitly {@link pb.SystemdUnit.verify|verify} messages.
         * @param message SystemdUnit message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ISystemdUnit, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a SystemdUnit message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns SystemdUnit
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.SystemdUnit;

        /**
         * Decodes a SystemdUnit message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns SystemdUnit
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.SystemdUnit;

        /**
         * Verifies a SystemdUnit message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a SystemdUnit message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns SystemdUnit
         */
        public static fromObject(object: { [k: string]: any }): pb.SystemdUnit;

        /**
         * Creates a plain object from a SystemdUnit message. Also converts values to other types if specified.
         * @param message SystemdUnit
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.SystemdUnit, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this SystemdUnit to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for SystemdUnit
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a SystemdUnitList. */
    interface ISystemdUnitList {

        /** SystemdUnitList services */
        services?: (pb.ISystemdUnit[]|null);
    }

    /** Represents a SystemdUnitList. */
    class SystemdUnitList implements ISystemdUnitList {

        /**
         * Constructs a new SystemdUnitList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ISystemdUnitList);

        /** SystemdUnitList services. */
        public services: pb.ISystemdUnit[];

        /**
         * Creates a new SystemdUnitList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns SystemdUnitList instance
         */
        public static create(properties?: pb.ISystemdUnitList): pb.SystemdUnitList;

        /**
         * Encodes the specified SystemdUnitList message. Does not implicitly {@link pb.SystemdUnitList.verify|verify} messages.
         * @param message SystemdUnitList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ISystemdUnitList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified SystemdUnitList message, length delimited. Does not implicitly {@link pb.SystemdUnitList.verify|verify} messages.
         * @param message SystemdUnitList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ISystemdUnitList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a SystemdUnitList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns SystemdUnitList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.SystemdUnitList;

        /**
         * Decodes a SystemdUnitList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns SystemdUnitList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.SystemdUnitList;

        /**
         * Verifies a SystemdUnitList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a SystemdUnitList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns SystemdUnitList
         */
        public static fromObject(object: { [k: string]: any }): pb.SystemdUnitList;

        /**
         * Creates a plain object from a SystemdUnitList message. Also converts values to other types if specified.
         * @param message SystemdUnitList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.SystemdUnitList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this SystemdUnitList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for SystemdUnitList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a SensorReading. */
    interface ISensorReading {

        /** SensorReading key */
        key?: (string|null);

        /** SensorReading value */
        value?: (number|null);

        /** SensorReading unit */
        unit?: (string|null);
    }

    /** Represents a SensorReading. */
    class SensorReading implements ISensorReading {

        /**
         * Constructs a new SensorReading.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ISensorReading);

        /** SensorReading key. */
        public key: string;

        /** SensorReading value. */
        public value: number;

        /** SensorReading unit. */
        public unit: string;

        /**
         * Creates a new SensorReading instance using the specified properties.
         * @param [properties] Properties to set
         * @returns SensorReading instance
         */
        public static create(properties?: pb.ISensorReading): pb.SensorReading;

        /**
         * Encodes the specified SensorReading message. Does not implicitly {@link pb.SensorReading.verify|verify} messages.
         * @param message SensorReading message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ISensorReading, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified SensorReading message, length delimited. Does not implicitly {@link pb.SensorReading.verify|verify} messages.
         * @param message SensorReading message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ISensorReading, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a SensorReading message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns SensorReading
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.SensorReading;

        /**
         * Decodes a SensorReading message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns SensorReading
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.SensorReading;

        /**
         * Verifies a SensorReading message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a SensorReading message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns SensorReading
         */
        public static fromObject(object: { [k: string]: any }): pb.SensorReading;

        /**
         * Creates a plain object from a SensorReading message. Also converts values to other types if specified.
         * @param message SensorReading
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.SensorReading, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this SensorReading to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for SensorReading
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a SensorsSnapshot. */
    interface ISensorsSnapshot {

        /** SensorsSnapshot temperatures */
        temperatures?: (pb.ISensorReading[]|null);

        /** SensorsSnapshot fans */
        fans?: (string[]|null);
    }

    /** Represents a SensorsSnapshot. */
    class SensorsSnapshot implements ISensorsSnapshot {

        /**
         * Constructs a new SensorsSnapshot.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ISensorsSnapshot);

        /** SensorsSnapshot temperatures. */
        public temperatures: pb.ISensorReading[];

        /** SensorsSnapshot fans. */
        public fans: string[];

        /**
         * Creates a new SensorsSnapshot instance using the specified properties.
         * @param [properties] Properties to set
         * @returns SensorsSnapshot instance
         */
        public static create(properties?: pb.ISensorsSnapshot): pb.SensorsSnapshot;

        /**
         * Encodes the specified SensorsSnapshot message. Does not implicitly {@link pb.SensorsSnapshot.verify|verify} messages.
         * @param message SensorsSnapshot message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ISensorsSnapshot, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified SensorsSnapshot message, length delimited. Does not implicitly {@link pb.SensorsSnapshot.verify|verify} messages.
         * @param message SensorsSnapshot message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ISensorsSnapshot, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a SensorsSnapshot message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns SensorsSnapshot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.SensorsSnapshot;

        /**
         * Decodes a SensorsSnapshot message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns SensorsSnapshot
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.SensorsSnapshot;

        /**
         * Verifies a SensorsSnapshot message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a SensorsSnapshot message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns SensorsSnapshot
         */
        public static fromObject(object: { [k: string]: any }): pb.SensorsSnapshot;

        /**
         * Creates a plain object from a SensorsSnapshot message. Also converts values to other types if specified.
         * @param message SensorsSnapshot
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.SensorsSnapshot, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this SensorsSnapshot to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for SensorsSnapshot
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ShellSession. */
    interface IShellSession {

        /** ShellSession id */
        id?: (string|null);

        /** ShellSession label */
        label?: (string|null);

        /** ShellSession pid */
        pid?: (number|null);

        /** ShellSession active */
        active?: (boolean|null);

        /** ShellSession createdAt */
        createdAt?: (number|Long|null);

        /** ShellSession cols */
        cols?: (string|null);

        /** ShellSession rows */
        rows?: (string|null);

        /** ShellSession cwd */
        cwd?: (string|null);
    }

    /** Represents a ShellSession. */
    class ShellSession implements IShellSession {

        /**
         * Constructs a new ShellSession.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IShellSession);

        /** ShellSession id. */
        public id: string;

        /** ShellSession label. */
        public label: string;

        /** ShellSession pid. */
        public pid: number;

        /** ShellSession active. */
        public active: boolean;

        /** ShellSession createdAt. */
        public createdAt: (number|Long);

        /** ShellSession cols. */
        public cols: string;

        /** ShellSession rows. */
        public rows: string;

        /** ShellSession cwd. */
        public cwd: string;

        /**
         * Creates a new ShellSession instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ShellSession instance
         */
        public static create(properties?: pb.IShellSession): pb.ShellSession;

        /**
         * Encodes the specified ShellSession message. Does not implicitly {@link pb.ShellSession.verify|verify} messages.
         * @param message ShellSession message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IShellSession, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ShellSession message, length delimited. Does not implicitly {@link pb.ShellSession.verify|verify} messages.
         * @param message ShellSession message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IShellSession, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ShellSession message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ShellSession
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ShellSession;

        /**
         * Decodes a ShellSession message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ShellSession
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ShellSession;

        /**
         * Verifies a ShellSession message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ShellSession message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ShellSession
         */
        public static fromObject(object: { [k: string]: any }): pb.ShellSession;

        /**
         * Creates a plain object from a ShellSession message. Also converts values to other types if specified.
         * @param message ShellSession
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ShellSession, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ShellSession to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ShellSession
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ShellSessionList. */
    interface IShellSessionList {

        /** ShellSessionList sessions */
        sessions?: (pb.IShellSession[]|null);
    }

    /** Represents a ShellSessionList. */
    class ShellSessionList implements IShellSessionList {

        /**
         * Constructs a new ShellSessionList.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IShellSessionList);

        /** ShellSessionList sessions. */
        public sessions: pb.IShellSession[];

        /**
         * Creates a new ShellSessionList instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ShellSessionList instance
         */
        public static create(properties?: pb.IShellSessionList): pb.ShellSessionList;

        /**
         * Encodes the specified ShellSessionList message. Does not implicitly {@link pb.ShellSessionList.verify|verify} messages.
         * @param message ShellSessionList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IShellSessionList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ShellSessionList message, length delimited. Does not implicitly {@link pb.ShellSessionList.verify|verify} messages.
         * @param message ShellSessionList message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IShellSessionList, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ShellSessionList message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ShellSessionList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ShellSessionList;

        /**
         * Decodes a ShellSessionList message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ShellSessionList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ShellSessionList;

        /**
         * Verifies a ShellSessionList message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ShellSessionList message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ShellSessionList
         */
        public static fromObject(object: { [k: string]: any }): pb.ShellSessionList;

        /**
         * Creates a plain object from a ShellSessionList message. Also converts values to other types if specified.
         * @param message ShellSessionList
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ShellSessionList, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ShellSessionList to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ShellSessionList
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ShellSessionCreateRequest. */
    interface IShellSessionCreateRequest {

        /** ShellSessionCreateRequest workdir */
        workdir?: (string|null);

        /** ShellSessionCreateRequest args */
        args?: (string[]|null);

        /** ShellSessionCreateRequest label */
        label?: (string|null);
    }

    /** Represents a ShellSessionCreateRequest. */
    class ShellSessionCreateRequest implements IShellSessionCreateRequest {

        /**
         * Constructs a new ShellSessionCreateRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IShellSessionCreateRequest);

        /** ShellSessionCreateRequest workdir. */
        public workdir: string;

        /** ShellSessionCreateRequest args. */
        public args: string[];

        /** ShellSessionCreateRequest label. */
        public label: string;

        /**
         * Creates a new ShellSessionCreateRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ShellSessionCreateRequest instance
         */
        public static create(properties?: pb.IShellSessionCreateRequest): pb.ShellSessionCreateRequest;

        /**
         * Encodes the specified ShellSessionCreateRequest message. Does not implicitly {@link pb.ShellSessionCreateRequest.verify|verify} messages.
         * @param message ShellSessionCreateRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IShellSessionCreateRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ShellSessionCreateRequest message, length delimited. Does not implicitly {@link pb.ShellSessionCreateRequest.verify|verify} messages.
         * @param message ShellSessionCreateRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IShellSessionCreateRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ShellSessionCreateRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ShellSessionCreateRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ShellSessionCreateRequest;

        /**
         * Decodes a ShellSessionCreateRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ShellSessionCreateRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ShellSessionCreateRequest;

        /**
         * Verifies a ShellSessionCreateRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ShellSessionCreateRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ShellSessionCreateRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.ShellSessionCreateRequest;

        /**
         * Creates a plain object from a ShellSessionCreateRequest message. Also converts values to other types if specified.
         * @param message ShellSessionCreateRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ShellSessionCreateRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ShellSessionCreateRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ShellSessionCreateRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a CapturedEventRecord. */
    interface ICapturedEventRecord {

        /** CapturedEventRecord event */
        event?: (pb.IEvent|null);

        /** CapturedEventRecord timestamp */
        timestamp?: (number|Long|null);
    }

    /** Represents a CapturedEventRecord. */
    class CapturedEventRecord implements ICapturedEventRecord {

        /**
         * Constructs a new CapturedEventRecord.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.ICapturedEventRecord);

        /** CapturedEventRecord event. */
        public event?: (pb.IEvent|null);

        /** CapturedEventRecord timestamp. */
        public timestamp: (number|Long);

        /**
         * Creates a new CapturedEventRecord instance using the specified properties.
         * @param [properties] Properties to set
         * @returns CapturedEventRecord instance
         */
        public static create(properties?: pb.ICapturedEventRecord): pb.CapturedEventRecord;

        /**
         * Encodes the specified CapturedEventRecord message. Does not implicitly {@link pb.CapturedEventRecord.verify|verify} messages.
         * @param message CapturedEventRecord message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.ICapturedEventRecord, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified CapturedEventRecord message, length delimited. Does not implicitly {@link pb.CapturedEventRecord.verify|verify} messages.
         * @param message CapturedEventRecord message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.ICapturedEventRecord, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a CapturedEventRecord message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns CapturedEventRecord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.CapturedEventRecord;

        /**
         * Decodes a CapturedEventRecord message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns CapturedEventRecord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.CapturedEventRecord;

        /**
         * Verifies a CapturedEventRecord message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a CapturedEventRecord message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns CapturedEventRecord
         */
        public static fromObject(object: { [k: string]: any }): pb.CapturedEventRecord;

        /**
         * Creates a plain object from a CapturedEventRecord message. Also converts values to other types if specified.
         * @param message CapturedEventRecord
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.CapturedEventRecord, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this CapturedEventRecord to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for CapturedEventRecord
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an EventHistoryResponse. */
    interface IEventHistoryResponse {

        /** EventHistoryResponse events */
        events?: (pb.ICapturedEventRecord[]|null);

        /** EventHistoryResponse source */
        source?: (string|null);
    }

    /** Represents an EventHistoryResponse. */
    class EventHistoryResponse implements IEventHistoryResponse {

        /**
         * Constructs a new EventHistoryResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IEventHistoryResponse);

        /** EventHistoryResponse events. */
        public events: pb.ICapturedEventRecord[];

        /** EventHistoryResponse source. */
        public source: string;

        /**
         * Creates a new EventHistoryResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EventHistoryResponse instance
         */
        public static create(properties?: pb.IEventHistoryResponse): pb.EventHistoryResponse;

        /**
         * Encodes the specified EventHistoryResponse message. Does not implicitly {@link pb.EventHistoryResponse.verify|verify} messages.
         * @param message EventHistoryResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IEventHistoryResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EventHistoryResponse message, length delimited. Does not implicitly {@link pb.EventHistoryResponse.verify|verify} messages.
         * @param message EventHistoryResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IEventHistoryResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EventHistoryResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EventHistoryResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.EventHistoryResponse;

        /**
         * Decodes an EventHistoryResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EventHistoryResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.EventHistoryResponse;

        /**
         * Verifies an EventHistoryResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EventHistoryResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EventHistoryResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.EventHistoryResponse;

        /**
         * Creates a plain object from an EventHistoryResponse message. Also converts values to other types if specified.
         * @param message EventHistoryResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.EventHistoryResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EventHistoryResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EventHistoryResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ConfigKeyRequest. */
    interface IConfigKeyRequest {

        /** ConfigKeyRequest key */
        key?: (string|null);

        /** ConfigKeyRequest name */
        name?: (string|null);
    }

    /** Represents a ConfigKeyRequest. */
    class ConfigKeyRequest implements IConfigKeyRequest {

        /**
         * Constructs a new ConfigKeyRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IConfigKeyRequest);

        /** ConfigKeyRequest key. */
        public key: string;

        /** ConfigKeyRequest name. */
        public name: string;

        /**
         * Creates a new ConfigKeyRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ConfigKeyRequest instance
         */
        public static create(properties?: pb.IConfigKeyRequest): pb.ConfigKeyRequest;

        /**
         * Encodes the specified ConfigKeyRequest message. Does not implicitly {@link pb.ConfigKeyRequest.verify|verify} messages.
         * @param message ConfigKeyRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IConfigKeyRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ConfigKeyRequest message, length delimited. Does not implicitly {@link pb.ConfigKeyRequest.verify|verify} messages.
         * @param message ConfigKeyRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IConfigKeyRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ConfigKeyRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ConfigKeyRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ConfigKeyRequest;

        /**
         * Decodes a ConfigKeyRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ConfigKeyRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ConfigKeyRequest;

        /**
         * Verifies a ConfigKeyRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ConfigKeyRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ConfigKeyRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.ConfigKeyRequest;

        /**
         * Creates a plain object from a ConfigKeyRequest message. Also converts values to other types if specified.
         * @param message ConfigKeyRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ConfigKeyRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ConfigKeyRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ConfigKeyRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ConfigBoolResponse. */
    interface IConfigBoolResponse {

        /** ConfigBoolResponse value */
        value?: (boolean|null);

        /** ConfigBoolResponse message */
        message?: (string|null);
    }

    /** Represents a ConfigBoolResponse. */
    class ConfigBoolResponse implements IConfigBoolResponse {

        /**
         * Constructs a new ConfigBoolResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IConfigBoolResponse);

        /** ConfigBoolResponse value. */
        public value: boolean;

        /** ConfigBoolResponse message. */
        public message: string;

        /**
         * Creates a new ConfigBoolResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ConfigBoolResponse instance
         */
        public static create(properties?: pb.IConfigBoolResponse): pb.ConfigBoolResponse;

        /**
         * Encodes the specified ConfigBoolResponse message. Does not implicitly {@link pb.ConfigBoolResponse.verify|verify} messages.
         * @param message ConfigBoolResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IConfigBoolResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ConfigBoolResponse message, length delimited. Does not implicitly {@link pb.ConfigBoolResponse.verify|verify} messages.
         * @param message ConfigBoolResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IConfigBoolResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ConfigBoolResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ConfigBoolResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.ConfigBoolResponse;

        /**
         * Decodes a ConfigBoolResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ConfigBoolResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.ConfigBoolResponse;

        /**
         * Verifies a ConfigBoolResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ConfigBoolResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ConfigBoolResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.ConfigBoolResponse;

        /**
         * Creates a plain object from a ConfigBoolResponse message. Also converts values to other types if specified.
         * @param message ConfigBoolResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.ConfigBoolResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ConfigBoolResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ConfigBoolResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }
}
